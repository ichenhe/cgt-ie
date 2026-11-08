// Package t212 parses Trading 212 "history" CSV exports into model.Trade.
package t212

import (
	"encoding/csv"
	"fmt"
	"io"
	"os"
	"strings"
	"time"

	"github.com/shopspring/decimal"

	"github.com/ichenhe/cgt-ie/internal/broker"
	"github.com/ichenhe/cgt-ie/internal/model"
)

// Name is the parser identifier used in Source.Broker and instrument keys.
const Name = "t212"

// Parser implements broker.Parser for Trading 212.
type Parser struct{}

var _ broker.Parser = Parser{}

// Name implements broker.Parser.
func (Parser) Name() string { return Name }

// Parse implements broker.Parser.
func (Parser) Parse(r io.Reader, file string) (broker.Result, error) { return Parse(r, file) }

// ParseFiles parses every path and merges the results.
func ParseFiles(paths []string) (broker.Result, error) {
	var parts []broker.Result
	for _, p := range paths {
		f, err := os.Open(p)
		if err != nil {
			return broker.Result{}, err
		}
		r, err := Parse(f, p)
		f.Close()
		if err != nil {
			return broker.Result{}, fmt.Errorf("%s: %w", p, err)
		}
		parts = append(parts, r)
	}
	return broker.Merge(parts...), nil
}

// feeColumns are the incidental-cost columns Trading 212 has used over time,
// each paired with its currency column. Exports only contain the columns
// that apply to the account, so some of these may be absent.
var feeColumns = []struct{ amount, ccy string }{
	{"Currency conversion fee", "Currency (Currency conversion fee)"},
	{"Stamp duty reserve tax", "Currency (Stamp duty reserve tax)"},
	{"Stamp duty", "Currency (Stamp duty)"},
	{"French transaction tax", "Currency (French transaction tax)"},
	{"Finra fee", "Currency (Finra fee)"},
	{"Transaction fee", "Currency (Transaction fee)"},
	{"Charge amount", "Currency (Charge amount)"},
}

// legacySuffixes are the account-currency suffixes older exports put on
// amount columns ("Total (EUR)", "Stamp duty reserve tax (GBP)").
var legacySuffixes = []string{" (EUR)", " (GBP)", " (USD)"}

func normaliseHeader(h string) (name, impliedCCY string) {
	h = clean(strings.TrimPrefix(h, "\ufeff")) // remove BOM header
	for _, suf := range legacySuffixes {
		if strings.HasSuffix(h, suf) {
			return strings.TrimSuffix(h, suf), strings.Trim(suf, " ()")
		}
	}
	return h, ""
}

// Parse reads one CSV export.
func Parse(r io.Reader, file string) (broker.Result, error) {
	var res broker.Result
	cr := csv.NewReader(r)
	cr.FieldsPerRecord = -1 // tolerate unexpected trailing columns
	cr.LazyQuotes = true

	header, err := cr.Read()
	if err != nil {
		return res, fmt.Errorf("read header: %w", err)
	}
	col := map[string]int{}
	implied := map[string]string{} // column → currency inferred from legacy suffix
	for i, h := range header {
		name, ccy := normaliseHeader(h)
		if _, dup := col[name]; dup {
			continue // prefer the first occurrence
		}
		col[name] = i
		if ccy != "" {
			implied[name] = ccy
		}
	}

	// check required columns
	for _, required := range []string{"Action", "ISIN", "No. of shares", "Total"} {
		if _, ok := col[required]; !ok {
			return res, fmt.Errorf("missing column %q from %v", required, header)
		}
	}
	timeCol, ok := col["Time (UTC)"]
	if !ok {
		timeCol, ok = col["Time"]
		if !ok {
			return res, fmt.Errorf("missing column \"Time (UTC)\" or \"Time\"")
		}
	}

	// get returns the cleaned value of a column, for the given row.
	get := func(row []string, name string) string {
		i, ok := col[name]
		if !ok || i >= len(row) {
			return ""
		}
		return clean(row[i])
	}

	line := 1
	index := 0
	for {
		row, err := cr.Read()
		if err == io.EOF {
			break
		}
		line++
		if err != nil {
			return res, fmt.Errorf("line %d: %w", line, err)
		}
		if len(row) == 0 || (len(row) == 1 && row[0] == "") {
			continue
		}
		src := model.Source{Broker: Name, File: file, Row: line, Index: index}
		action := get(row, "Action")
		kind := kindOf(action)
		if kind == model.Other {
			res.Skipped = append(res.Skipped, broker.Skipped{Source: src, Action: action})
			continue
		}
		index++

		t := model.Trade{
			ID:     get(row, "ID"),
			Kind:   kind,
			ISIN:   get(row, "ISIN"),
			Ticker: get(row, "Ticker"),
			Name:   get(row, "Name"),
			Source: src,
		}
		if t.Time, err = parseTime(clean(row[timeCol])); err != nil {
			return res, fmt.Errorf("line %d: %w", line, err)
		}
		if t.ISIN == "" {
			return res, fmt.Errorf("line %d: empty ISIN", line)
		}
		if t.Qty, err = dec(get(row, "No. of shares")); err != nil {
			return res, fmt.Errorf("line %d: No. of shares: %w", line, err)
		}
		if kind.IsTrade() && t.Qty.Sign() <= 0 {
			return res, fmt.Errorf("line %d: No. of shares must be > 0, got %s", line, t.Qty)
		}

		total, err := dec(get(row, "Total"))
		if err != nil {
			return res, fmt.Errorf("line %d: Total: %w", line, err)
		}
		total = total.Abs()
		settleCCY := get(row, "Currency (Total)")
		if settleCCY == "" {
			settleCCY = implied["Total"]
		}
		if settleCCY == "" {
			settleCCY = "EUR"
		}

		// Fees: each in its own currency; default to the settlement currency.
		for _, fc := range feeColumns {
			v, err := dec(get(row, fc.amount))
			if err != nil || v.IsZero() {
				continue
			}
			ccy := get(row, fc.ccy)
			if ccy == "" {
				ccy = implied[fc.amount]
			}
			if ccy == "" {
				ccy = settleCCY
			}
			t.Fees = append(t.Fees, model.Money{Amount: v.Abs(), CCY: ccy})
		}

		// Calculate the gross tx value: total - fees.
		//
		// For a fee in a different currency, not sure whether it is included in
		// Total, so leave it out of the adjustment for now and warn.
		for _, f := range t.Fees {
			if !strings.EqualFold(f.CCY, settleCCY) {
				res.Warnings = append(res.Warnings, fmt.Sprintf(
					"%s: %s %s has a %s fee but settled in %s; assumed NOT included in Total. Check this row by hand.", src, t.Day().Format("2006-01-02"), t.Ticker, f.CCY, settleCCY))
			}
		}
		feesInSettle := t.FeesTotalIn(settleCCY)
		gross := total
		switch kind {
		case model.Buy:
			gross = total.Sub(feesInSettle)
		case model.Sell:
			gross = total.Add(feesInSettle)
		}
		t.Gross = model.Money{Amount: gross, CCY: settleCCY}

		// Informational columns
		if p, err := dec(get(row, "Price / share")); err == nil {
			t.Price = model.Money{Amount: p, CCY: get(row, "Currency (Price / share)")}
		}
		t.BrokerFX, _ = dec(get(row, "Exchange rate"))
		// Result is only comparable to our EUR gain when it is in EUR; for
		// trades settled from a USD balance Trading 212 states it in USD.
		if cur := get(row, "Currency (Result)"); cur == "" || cur == "EUR" {
			if v, err := dec(get(row, "Result")); err == nil {
				t.BrokerResult = v
				t.HasBrokerResult = true
			}
		}

		res.Trades = append(res.Trades, t)
	}
	return res, nil
}

// kindOf maps the Action column. Trading 212 uses "Market buy", "Limit buy",
// "Stop buy", "Market sell", ..., "Dividend (Dividend)", "Dividend (Return
// of capital)", "Stock split open"/"Stock split close" (two rows per
// split), "Transfer in"/"Transfer out", "Stock distribution", and many
// cash-only actions (Deposit, Interest on cash, Card debit, Currency
// conversion, Spending cashback, ...).
func kindOf(action string) model.Kind {
	a := strings.ToLower(action)
	switch {
	case strings.HasSuffix(a, "buy"):
		return model.Buy
	case strings.HasSuffix(a, "sell"):
		return model.Sell
	case strings.HasPrefix(a, "dividend"):
		return model.Dividend
	case strings.Contains(a, "split"):
		return model.Split
	case a == "transfer in":
		return model.TransferIn
	case a == "transfer out":
		return model.TransferOut
	case strings.Contains(a, "stock distribution"):
		return model.TransferIn
	}
	return model.Other
}

var timeLayouts = []string{
	"2006-01-02 15:04:05-07:00", // 2025-02-03 08:00:02+00:00
	"2006-01-02 15:04:05.000",   // older exports: 2024-01-01 00:00:00.000
	"2006-01-02 15:04:05",
	time.RFC3339,
}

func parseTime(s string) (time.Time, error) {
	for _, l := range timeLayouts {
		if t, err := time.Parse(l, s); err == nil {
			return t.UTC(), nil
		}
	}
	return time.Time{}, fmt.Errorf("unrecognised time %q", s)
}

// dec parses a decimal, treating empty as an error.
func dec(s string) (decimal.Decimal, error) {
	if s == "" {
		return decimal.Zero, fmt.Errorf("empty")
	}
	s = strings.ReplaceAll(s, ",", "")
	return decimal.NewFromString(s)
}

// clean normalizes spaces and trims them.
func clean(s string) string {
	s = strings.ReplaceAll(s, "\u00a0", " ")
	return strings.TrimSpace(s)
}
