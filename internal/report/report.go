// Package report renders engine results as Markdown and a flat CSV of every lot
// match for audit.
package report

import (
	"encoding/csv"
	"fmt"
	"io"
	"sort"
	"strings"
	"time"

	"github.com/shopspring/decimal"

	"github.com/ichenhe/cgt-ie/internal/config"
	"github.com/ichenhe/cgt-ie/internal/lots"
	"github.com/ichenhe/cgt-ie/internal/model"
	"github.com/ichenhe/cgt-ie/internal/tax/cgt"
	"github.com/ichenhe/cgt-ie/internal/tax/exittax"
)

// Data is everything the report needs.
type Data struct {
	Year                   int // 0 = all years
	CGT                    *cgt.Result
	ExitTax                *exittax.Result
	Instruments            map[string]model.Instrument
	Warnings               []string
	GeneratedAt            time.Time
	LossRestrictionApplied bool
	Rules                  config.Rules
	SourceFiles            []string
}

// YearSpan returns the first and last tax year with a disposal or a deemed
// disposal already due, after applying the Year filter. ok is false if there
// is nothing to report.
func (dt Data) YearSpan() (first, last int, ok bool) {
	add := func(y int) {
		if dt.Year != 0 && y != dt.Year {
			return
		}
		if !ok || y < first {
			first = y
		}
		if !ok || y > last {
			last = y
		}
		ok = true
	}
	if dt.CGT != nil {
		for _, y := range dt.CGT.Years {
			add(y.Year)
		}
	}
	if dt.ExitTax != nil {
		for _, y := range dt.ExitTax.Years {
			add(y.Year)
		}
	}
	return
}

func eur(d decimal.Decimal) string {
	return "€" + d.StringFixed(2)
}

func neg(d decimal.Decimal) string {
	if d.IsZero() {
		return "€0.00"
	}
	return "−€" + d.StringFixed(2)
}

func date(t time.Time) string { return t.Format("2006-01-02") }

func (dt Data) name(key string) string {
	if in, ok := dt.Instruments[key]; ok && in.Ticker != "" {
		return in.Ticker
	}
	return key
}

// Markdown writes the full report.
func Markdown(w io.Writer, dt Data) {
	p := func(format string, a ...any) { fmt.Fprintf(w, format, a...) }

	p("# Irish CGT / exit tax Report\n\n")
	sourceFileUl := make([]string, len(dt.SourceFiles))
	for i, f := range dt.SourceFiles {
		sourceFileUl[i] = fmt.Sprintf("- `%s`", f)
	}
	p("Generated at %s. Sources:\n\n%s\n\n", dt.GeneratedAt.Format("2006-01-02 15:04 UTC"), strings.Join(sourceFileUl, "\n"))
	p("> This is a calculation aid, not tax advice. Check every figure before filing.\n\n")

	if len(dt.Warnings) > 0 {
		p("## Warnings\n\n")
		for _, s := range dt.Warnings {
			p("- %s\n", s)
		}
		p("\n")
	}

	writeCGT(w, dt)
	writeExitTax(w, dt)
}

func writeCGT(w io.Writer, dt Data) {
	p := func(format string, a ...any) { fmt.Fprintf(w, format, a...) }
	r := dt.CGT
	if r == nil || len(r.Years) == 0 {
		p("## CGT (shares)\n\nNo disposals.\n\n")
		return
	}
	p("## CGT (shares)\n\n")

	p("s.581 matching: %d-day LIFO then FIFO.\n\n", dt.Rules.CGT.MatchingWindow)

	if dt.LossRestrictionApplied {
		windowState := ""
		p("s.581(3) loss restriction: **applied%s** with a %d-day window.\n\n", windowState, dt.Rules.CGT.ReacquisitionWindow)
	} else {
		p("s.581(3) loss restriction: **not applied**.\n\n")
	}

	p("%s annual exemption. Rate by disposal date:\n\n", eur(dt.Rules.CGT.AnnualExemption))
	writeRateTable(w, dt.Rules.CGT.Rates)

	p("### Tax year summary\n\n")

	p("| Year | Proceeds | Cost | Chargeable gains | Allowable losses | Losses fwd in | Net gain | Exemption | Taxable | Rate | **Tax** | Due 15 Dec | Due 31 Jan | Losses fwd out | Restricted (s.581(3)) |\n")
	p("|---|---:|---:|---:|---:|---:|---:|---:|---:|---|---:|---:|---:|---:|---:|\n")
	for _, y := range r.Years {
		if dt.Year != 0 && y.Year != dt.Year {
			continue
		}
		p("| %d | %s | %s | %s | %s | %s | %s | %s | %s | %s | **%s** | %s | %s | %s | %s |\n",
			y.Year, eur(y.ProceedsEUR), eur(y.CostEUR), eur(y.ChargeableGainsEUR), neg(y.AllowableLossesEUR),
			neg(y.LossesForwardInEUR), eur(y.NetGainEUR), neg(y.ExemptionUsedEUR), eur(y.TaxableEUR), rateCell(y.TaxableByRate), eur(y.TaxEUR),
			eur(y.InitialPeriodTaxEUR), eur(y.LaterPeriodTaxEUR), eur(y.LossesForwardOutEUR), eur(y.RestrictedLossesEUR))
	}
	p("\n")

	for _, y := range r.Years {
		if dt.Year != 0 && y.Year != dt.Year {
			continue
		}
		if len(y.Disposals) == 0 {
			continue
		}
		p("### %d disposals\n\n", y.Year)
		p("| Date | Instrument | Qty | Proceeds | Cost | Gain/loss | Chargeable | Allowable loss | Restricted | Notes |\n")
		p("|---|---|---:|---:|---:|---:|---:|---:|---:|---|\n")
		for _, d := range y.Disposals {
			p("| %s | %s | %s | %s | %s | %s | %s | %s | %s | %s |\n",
				date(d.Trade.Day()), dt.name(d.Trade.Key()), d.Trade.Qty.String(),
				eur(d.ProceedsEUR), eur(d.CostEUR), signed(d.RawGainEUR),
				eur(d.ChargeableGainEUR), neg(d.AllowableLossEUR), eur(d.RestrictedLossEUR),
				strings.Join(d.Notes, "; "))
		}
		p("\n")
	}

	writeHoldings(w, "Open CGT holdings", r.Books, dt)
}

func writeExitTax(w io.Writer, dt Data) {
	p := func(format string, a ...any) { fmt.Fprintf(w, format, a...) }
	r := dt.ExitTax
	if r == nil || (len(r.Years) == 0 && len(r.Deemed) == 0) {
		p("## Exit tax (EU UCITS funds / ETFs)\n\nNo trades.\n\n")
		return
	}
	p("## Exit tax (EU UCITS ETFs)\n\n")
	p("Losses are not allowable and are not netted against anything. Report under *Offshore funds* on Form 11 / CG1; pay with your income tax return (31 Oct of the following year).\n\n")
	p("Rate by date of the chargeable event:\n\n")
	writeRateTable(w, dt.Rules.ExitTax.Rates)

	p("### Tax year summary\n\n")
	if len(r.Years) > 0 {
		p("| Year | Proceeds | Cost | Taxable gains | Disallowed losses | **Tax** | Due |\n")
		p("|---|---:|---:|---:|---:|---:|---|\n")
		for _, y := range r.Years {
			if dt.Year != 0 && y.Year != dt.Year {
				continue
			}
			p("| %d | %s | %s | %s | %s | **%s** | %s |\n", y.Year, eur(y.ProceedsEUR), eur(y.CostEUR),
				eur(y.GainsEUR), neg(y.DisallowedLosses), eur(y.TaxEUR), date(y.DueDate))
		}
		p("\n")
		for _, y := range r.Years {
			if dt.Year != 0 && y.Year != dt.Year {
				continue
			}
			p("### %d disposals\n\n", y.Year)
			p("| Date | Fund | Qty | Proceeds | Cost | Gain/loss | Rate | Tax | Notes |\n")
			p("|---|---|---:|---:|---:|---:|---:|---:|---|\n")
			for _, d := range y.Disposals {
				p("| %s | %s | %s | %s | %s | %s | %s%% | %s | %s |\n",
					date(d.Trade.Day()), dt.name(d.Trade.Key()), d.Trade.Qty.String(),
					eur(d.ProceedsEUR), eur(d.CostEUR), signed(d.GainEUR),
					d.Rate.Mul(decimal.NewFromInt(100)).String(), eur(d.TaxEUR), strings.Join(d.Notes, "; "))
			}
			p("\n")
		}
	}

	if len(r.Deemed) > 0 {
		p("### Deemed disposals (8-year rule)\n\n")
		p("Each open lot is treated as sold on the 8th anniversary of purchase. You need the fund's price on that date to compute the gain; tax already paid on a deemed disposal is credited on the real sale.\n\n")
		p("| Date | Fund | Bought | Qty | Cost | Rate | Status |\n")
		p("|---|---|---|---:|---:|---:|---|\n")
		for _, d := range r.Deemed {
			status := "upcoming"
			if d.Past {
				status = "**PAST — report with price on this date**"
			}
			p("| %s | %s | %s | %s | %s | %s%% | %s |\n", date(d.Date), dt.name(d.Lot.Key), date(d.Lot.AcquiredAt),
				d.Qty.String(), eur(d.CostEUR), d.Rate.Mul(decimal.NewFromInt(100)).String(), status)
		}
		p("\n")
	}

	writeHoldings(w, "Open exit-tax holdings", r.Books, dt)
}

func writeHoldings(w io.Writer, title string, books map[string]*lots.Book, dt Data) {
	p := func(format string, a ...any) { fmt.Fprintf(w, format, a...) }
	var isins []string
	for isin, b := range books {
		if b.HeldQty().Sign() > 0 {
			isins = append(isins, isin)
		}
	}
	if len(isins) == 0 {
		return
	}
	sort.Slice(isins, func(i, j int) bool { return dt.name(isins[i]) < dt.name(isins[j]) })
	p("### %s\n\n", title)
	p("Holdings at the end of the provided exports.\n\n")
	p("| Instrument | Qty held | Cost basis | Open lots | Restricted loss carried |\n")
	p("|---|---:|---:|---:|---:|\n")
	for _, isin := range isins {
		b := books[isin]
		cost, restricted := decimal.Zero, decimal.Zero
		open := b.Open()
		for _, l := range open {
			cost = cost.Add(l.CostEUR)
			restricted = restricted.Add(l.RestrictedLossEUR)
		}
		p("| %s | %s | %s | %d | %s |\n", dt.name(isin), b.HeldQty().String(), eur(cost), len(open), eur(restricted))
	}
	p("\n")
}

// writeRateTable renders a step table of rate bands as "from – to  rate", one
// row per band. Bands must be sorted by From (LoadRules does this). The last
// band has no end.
func writeRateTable(w io.Writer, bands []config.RateBand) {
	p := func(format string, a ...any) { fmt.Fprintf(w, format, a...) }
	if len(bands) == 0 {
		p("_No rates configured._\n\n")
		return
	}
	p("| From | To | Tax Rate |\n")
	p("|---|---|---:|\n")
	for i, b := range bands {
		to := "onwards"
		if i+1 < len(bands) {
			to = date(bands[i+1].From.AddDate(0, 0, -1))
		}
		p("| %s | %s | %s |\n", date(b.From), to, pct(b.Rate))
	}
	p("\n")
}

func pct(r decimal.Decimal) string { return r.Mul(decimal.NewFromInt(100)).String() + "%" }

// rateCell renders the rate(s) applied: "33%" or "33% on €1530.00 + 31% on €1500.00".
func rateCell(slices []cgt.RateSlice) string {
	if len(slices) == 0 {
		return "—"
	}
	if len(slices) == 1 {
		return pct(slices[0].Rate)
	}
	parts := make([]string, 0, len(slices))
	for _, s := range slices {
		parts = append(parts, pct(s.Rate)+" on "+eur(s.TaxableEUR))
	}
	return strings.Join(parts, " + ")
}

func signed(d decimal.Decimal) string {
	if d.Sign() < 0 {
		return "−€" + d.Neg().StringFixed(2)
	}
	return "€" + d.StringFixed(2)
}

// SalesBreakdownCSV writes one row per lot match across both regimes.
func SalesBreakdownCSV(w io.Writer, dt Data) error {
	cw := csv.NewWriter(w)
	defer cw.Flush()
	if err := cw.Write([]string{
		"regime", "disposal_date", "disposal_id", "isin", "ticker", "rule",
		"lot_acquired", "lot_trade_id", "qty", "cost_eur", "proceeds_eur", "gain_eur",
		"lot_fx_note", "disposal_fx_note",
	}); err != nil {
		return err
	}
	type row struct {
		t    time.Time
		cols []string
	}
	var rows []row
	if dt.CGT != nil {
		for _, d := range dt.CGT.Disposals {
			if dt.Year != 0 && d.Year() != dt.Year {
				continue
			}
			for _, m := range d.Matches {
				rows = append(rows, row{d.Trade.Time, []string{
					"cgt", date(d.Trade.Day()), d.Trade.ID, d.Trade.Key(), dt.name(d.Trade.Key()), string(m.Rule),
					date(m.Lot.AcquiredAt), m.Lot.TradeID, m.Qty.String(),
					m.CostEUR.StringFixed(4), m.ProceedsEUR.StringFixed(4), m.GainEUR().StringFixed(4),
					m.Lot.FXNote, d.Trade.FXNote,
				}})
			}
		}
	}
	if dt.ExitTax != nil {
		for _, d := range dt.ExitTax.Disposals {
			if dt.Year != 0 && d.Year() != dt.Year {
				continue
			}
			for _, m := range d.Matches {
				rows = append(rows, row{d.Trade.Time, []string{
					"exit_tax", date(d.Trade.Day()), d.Trade.ID, d.Trade.Key(), dt.name(d.Trade.Key()), string(m.Rule),
					date(m.Lot.AcquiredAt), m.Lot.TradeID, m.Qty.String(),
					m.CostEUR.StringFixed(4), m.ProceedsEUR.StringFixed(4), m.GainEUR().StringFixed(4),
					m.Lot.FXNote, d.Trade.FXNote,
				}})
			}
		}
	}
	sort.SliceStable(rows, func(i, j int) bool { return rows[i].t.Before(rows[j].t) })
	for _, r := range rows {
		if err := cw.Write(r.cols); err != nil {
			return err
		}
	}
	return nil
}
