// Package broker defines the contract every broker-export parser implements
// and the shared result types.
package broker

import (
	"fmt"
	"io"
	"sort"
	"strings"
	"time"

	"github.com/shopspring/decimal"

	"github.com/ichenhe/cgt-ie/internal/fx"
	"github.com/ichenhe/cgt-ie/internal/model"
)

// Skipped describes a row that was read but not turned into a trade
// (deposits, interest, card spend, ...).
type Skipped struct {
	Source model.Source
	Action string
}

// Result is the output of parsing one or more files.
type Result struct {
	Trades  []model.Trade
	Skipped []Skipped
	// Duplicates is the number of rows dropped because the same trade had
	// already been seen.
	Duplicates int
	// Warnings are cases parser handled on an assumption, rather than a solid
	// rule.
	Warnings []string
}

type Parser interface {
	// Name is a short identifier such as "t212".
	Name() string
	Parse(r io.Reader, file string) (Result, error)
}

// Converter supplies EUR reference rates for non-EUR amounts.
type Converter interface {
	ToEUR(amount decimal.Decimal, ccy string, date time.Time) (decimal.Decimal, fx.Lookup, error)
}

// ConvertToEUR fills TotalEUR for every trade from Gross and Fees:
// buy  → gross + fees, sell → gross − fees, each component converted to EUR
// at its own currency's rate on the trade date. It returns how many trades
// were converted. conv may be nil if no trade needs it.
func ConvertToEUR(trades []model.Trade, conv Converter) (int, error) {
	n := 0
	for i := range trades {
		t := &trades[i]
		if t.Kind != model.Buy && t.Kind != model.Sell {
			continue
		}
		var notes []string
		toEUR := func(m model.Money) (decimal.Decimal, error) {
			if m.IsEUR() {
				return m.Amount, nil
			}
			if conv == nil {
				return decimal.Zero, fmt.Errorf("%s: %s %s settled in %s; an converter is required",
					t.Source, t.Day().Format("2006-01-02"), t.Ticker, m.CCY)
			}
			eur, lk, err := conv.ToEUR(m.Amount, m.CCY, t.Day())
			if err != nil {
				return decimal.Zero, fmt.Errorf("%s (%s %s): %w", t.Source, t.Ticker, t.Day().Format("2006-01-02"), err)
			}
			note := fmt.Sprintf("%s @ ECB %s", m, lk.Rate)
			if lk.Fallback {
				note += " (" + lk.Date.Format("2006-01-02") + ", previous business day)"
			}
			notes = append(notes, note)
			return eur, nil
		}

		total, err := toEUR(t.Gross)
		if err != nil {
			return n, err
		}
		for _, f := range t.Fees {
			fe, err := toEUR(f)
			if err != nil {
				return n, err
			}
			if t.Kind == model.Buy {
				total = total.Add(fe)
			} else {
				total = total.Sub(fe)
			}
		}
		t.TotalEUR = total.Round(2)
		if len(notes) > 0 {
			t.FXNote = strings.Join(notes, "; ")
			n++
		}
	}
	return n, nil
}

// Merge combines several results: synthesises missing ID if necessary,
// de-duplicates by ID across inputs, and sorts by time, then by the order
// the parser read the rows, then by ID.
func Merge(results ...Result) Result {
	var out Result
	seen := map[string]model.Source{} // ID → where it was first seen
	for _, r := range results {
		for _, t := range r.Trades {
			synthetic := t.ID == ""
			if synthetic {
				t.ID = syntheticID(t)
			}
			if first, dup := seen[t.ID]; dup {
				out.Duplicates++
				if synthetic && first.File == t.Source.File {
					out.Warnings = append(out.Warnings, fmt.Sprintf(
						"%s: identical to row %d of the same file (%s %s %s %s); treated as a duplicate. If these are two separate trades, this broker's export cannot distinguish them — add the second one by hand.",
						t.Source, first.Row, t.Day().Format("2006-01-02"), t.Kind, t.Qty, t.Gross))
				}
				continue
			}
			seen[t.ID] = t.Source
			out.Trades = append(out.Trades, t)
		}
		out.Skipped = append(out.Skipped, r.Skipped...)
		out.Duplicates += r.Duplicates
		out.Warnings = append(out.Warnings, r.Warnings...)
	}
	sort.SliceStable(out.Trades, func(i, j int) bool {
		a, b := out.Trades[i], out.Trades[j]
		if !a.Time.Equal(b.Time) {
			return a.Time.Before(b.Time)
		}
		if a.Source.File == b.Source.File && a.Source.Index != b.Source.Index {
			return a.Source.Index < b.Source.Index
		}
		return a.ID < b.ID
	})
	return out
}

// syntheticID builds a stable ID from the row's content for brokers that do
// not export one.
func syntheticID(t model.Trade) string {
	return fmt.Sprintf("%s|%s|%s|%s|%s|%s%s", t.Source.Broker, t.Time.UTC().Format(time.RFC3339),
		t.Kind, t.Key(), t.Qty.String(), t.Gross.CCY, t.Gross.Amount.String())
}
