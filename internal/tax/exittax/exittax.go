// Package exittax computes Irish exit tax on disposals of EU/EEA/OECD
// "equivalent" offshore funds (TCA 1997 s.747E), which is how Irish- and
// Luxembourg-domiciled UCITS ETFs are taxed, and how US-domiciled ETFs are
// treated by default since Revenue withdrew its shares-like treatment from
// 1 January 2022 (TDM 27-01A-03). From 1 January 2027 Irish ETFs move to
// s.739DA (Finance (No. 2) Bill 2026), which keeps the same rate, FIFO and
// loss rules.
//
// Rules implemented:
//   - Identification: FIFO only.
//   - Gains are taxed at the rate in force on the disposal date, as income
//     under Case IV. No annual exemption.
//   - Losses are disallowed: they cannot be set against a gain on other funds.
//   - Deemed disposal: every lot is treated as disposed of on the 8th
//     anniversary of acquisition. This engine lists them only but does not
//     calculate the tax, because the fund's price on that date is unknown.
//     Lots keep their original cost afterwards; the credit for tax paid on a
//     deemed disposal is not tracked.
//   - Payment: via Form 11 "Offshore funds" panel, due with the income tax
//     return (31 Oct of the following year), not on the CGT dates.
package exittax

import (
	"fmt"
	"sort"
	"time"

	"github.com/shopspring/decimal"

	"github.com/ichenhe/cgt-ie/internal/config"
	"github.com/ichenhe/cgt-ie/internal/lots"
	"github.com/ichenhe/cgt-ie/internal/model"
)

// Input to Compute.
type Input struct {
	// Trades under the exit tax regime, sorted by time ASC.
	Trades []model.Trade
	Rules  config.ExitTaxRules
	// Now is used to split "deemed disposals" into past (overdue, must be
	// reported) and upcoming.
	Now time.Time
}

// Disposal is one sell trade with all related information and tax due.
type Disposal struct {
	Trade   model.Trade
	Matches []lots.Match

	ProceedsEUR decimal.Decimal // Σ Matches.ProceedsEUR
	CostEUR     decimal.Decimal // Σ Matches.CostEUR
	GainEUR     decimal.Decimal // proceeds − cost, may be <0
	TaxableEUR  decimal.Decimal // max(GainEUR, 0)
	Rate        decimal.Decimal
	TaxEUR      decimal.Decimal
	Notes       []string
}

// Year returns the tax year of the disposal.
func (d *Disposal) Year() int { return d.Trade.Day().Year() }

// Deemed is a "deemed disposal" event for one lot.
type Deemed struct {
	Lot     *lots.Lot
	Date    time.Time // acquisition + N years
	Qty     decimal.Decimal
	CostEUR decimal.Decimal
	Rate    decimal.Decimal
	Past    bool // Date <= Now
}

// YearSummary aggregates one tax year.
type YearSummary struct {
	Year      int
	Disposals []*Disposal

	ProceedsEUR      decimal.Decimal
	CostEUR          decimal.Decimal
	GainsEUR         decimal.Decimal // Σ positive gains
	DisallowedLosses decimal.Decimal // Σ losses (>= 0), informational
	TaxEUR           decimal.Decimal
	DueDate          time.Time // 31 Oct following year
}

// Result of Compute.
type Result struct {
	Disposals []*Disposal
	Years     []*YearSummary
	Deemed    []Deemed
	Books     map[string]*lots.Book
	Warnings  []string
}

// Compute runs the exit tax calculation.
func Compute(in Input) (*Result, error) {
	res := &Result{Books: map[string]*lots.Book{}}
	for _, t := range in.Trades {
		book, ok := res.Books[t.Key()]
		if !ok {
			book = lots.NewBook(t.Key())
			res.Books[t.Key()] = book
		}
		switch t.Kind {
		case model.Buy:
			book.Acquire(t)
		case model.Sell:
			matches, err := book.Dispose(t, 0)
			if err != nil {
				return nil, err
			}
			d := &Disposal{Trade: t, Matches: matches}
			if t.FXNote != "" {
				d.Notes = append(d.Notes, "proceeds: "+t.FXNote)
			}
			for _, m := range matches {
				d.ProceedsEUR = d.ProceedsEUR.Add(m.ProceedsEUR)
				d.CostEUR = d.CostEUR.Add(m.CostEUR)
			}
			d.GainEUR = d.ProceedsEUR.Sub(d.CostEUR)
			if d.GainEUR.Sign() > 0 {
				d.TaxableEUR = d.GainEUR
			} else if d.GainEUR.Sign() < 0 {
				d.Notes = append(d.Notes, "loss on an offshore fund is not allowable (s.747E); nothing to set off or carry forward")
			}
			rate, err := in.Rules.RateOn(t.Day())
			if err != nil {
				return nil, fmt.Errorf("trade %s: %w", t.ID, err)
			}
			d.Rate = rate
			d.TaxEUR = d.TaxableEUR.Mul(rate).Round(2)
			res.Disposals = append(res.Disposals, d)
		default:
			return nil, fmt.Errorf("trade %s: unexpected kind %q", t.ID, t.Kind)
		}
	}

	res.Years = summarise(res.Disposals)
	res.Deemed = deemedSchedule(res.Books, in)
	if n := countPast(res.Deemed); n > 0 {
		res.Warnings = append(res.Warnings, fmt.Sprintf(
			"%d deemed disposal(s) have already occurred; you should report them with the fund's price on those dates", n))
	}
	return res, nil
}

func summarise(disposals []*Disposal) []*YearSummary {
	byYear := map[int]*YearSummary{}
	for _, d := range disposals {
		y := d.Year()
		ys, ok := byYear[y]
		if !ok {
			ys = &YearSummary{Year: y, DueDate: time.Date(y+1, time.October, 31, 0, 0, 0, 0, time.UTC)}
			byYear[y] = ys
		}
		ys.Disposals = append(ys.Disposals, d)
		ys.ProceedsEUR = ys.ProceedsEUR.Add(d.ProceedsEUR)
		ys.CostEUR = ys.CostEUR.Add(d.CostEUR)
		ys.GainsEUR = ys.GainsEUR.Add(d.TaxableEUR)
		if d.GainEUR.Sign() < 0 {
			ys.DisallowedLosses = ys.DisallowedLosses.Add(d.GainEUR.Neg())
		}
		ys.TaxEUR = ys.TaxEUR.Add(d.TaxEUR)
	}
	var out []*YearSummary
	for _, ys := range byYear {
		out = append(out, ys)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Year < out[j].Year })
	return out
}

// deemedSchedule lists the 8-year anniversaries of every lot that is still
// (at least partly) open, in date order.
func deemedSchedule(books map[string]*lots.Book, in Input) []Deemed {
	var out []Deemed
	now := model.Day(in.Now)
	for _, b := range books {
		for _, l := range b.Open() {
			date := l.AcquiredAt.AddDate(in.Rules.DeemedDisposalYrs, 0, 0)
			rate, err := in.Rules.RateOn(date)
			if err != nil {
				rate = decimal.Zero
			}
			out = append(out, Deemed{
				Lot:     l,
				Date:    date,
				Qty:     l.Qty,
				CostEUR: l.CostEUR,
				Rate:    rate,
				Past:    !date.After(now),
			})
		}
	}
	sort.Slice(out, func(i, j int) bool {
		if !out[i].Date.Equal(out[j].Date) {
			return out[i].Date.Before(out[j].Date)
		}
		return out[i].Lot.TradeID < out[j].Lot.TradeID
	})
	return out
}

func countPast(ds []Deemed) int {
	n := 0
	for _, d := range ds {
		if d.Past {
			n++
		}
	}
	return n
}
