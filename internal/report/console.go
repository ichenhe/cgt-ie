package report

import (
	"fmt"
	"io"

	"github.com/jedib0t/go-pretty/v6/table"
	"github.com/jedib0t/go-pretty/v6/text"

	"github.com/ichenhe/cgt-ie/internal/tax/exittax"
)

// Console writes a short summary of the report to w.
func Console(w io.Writer, dt Data) {
	p := func(format string, a ...any) { fmt.Fprintf(w, format, a...) }

	p("\n")
	if len(dt.Warnings) > 0 {
		p("Warnings\n")
		for _, s := range dt.Warnings {
			p("  - %s\n", s)
		}
		p("\n")
	}

	consoleCGT(w, dt)
	consoleExitTax(w, dt)

	p("\nSee generated report for full details.\n")
}

func newTable(w io.Writer, title string) table.Writer {
	t := table.NewWriter()
	t.SetOutputMirror(w)
	t.SetStyle(table.StyleLight)
	t.Style().Format.Header = text.FormatDefault
	t.Style().Title.Align = text.AlignLeft
	t.SetTitle(title)
	return t
}

// rightAlign marks every column except the first as right-aligned.
func rightAlign(t table.Writer, n int, skipCols ...int) {
	skip := map[int]struct{}{1: {}}
	for _, i := range skipCols {
		skip[i] = struct{}{}
	}
	var cfgs []table.ColumnConfig
	for i := 1; i <= n; i++ {
		if _, ok := skip[i]; ok {
			continue
		}
		cfgs = append(cfgs, table.ColumnConfig{Number: i, Align: text.AlignRight, AlignHeader: text.AlignRight})
	}
	t.SetColumnConfigs(cfgs)
}

func consoleCGT(w io.Writer, dt Data) {
	r := dt.CGT
	if r == nil || len(r.Years) == 0 {
		fmt.Fprintf(w, "CGT (shares): no disposals.\n\n")
		return
	}
	t := newTable(w, "CGT (shares)")
	t.AppendHeader(table.Row{"Year", "Gains", "Losses", "Net gain", "Taxable", "Tax", "Due 15 Dec / 31 Jan", "Losses fwd in / out"})
	rightAlign(t, 8)
	for _, y := range r.Years {
		if dt.Year != 0 && y.Year != dt.Year {
			continue
		}
		t.AppendRow(table.Row{
			y.Year, eur(y.ChargeableGainsEUR), neg(y.AllowableLossesEUR), eur(y.NetGainEUR), eur(y.TaxableEUR),
			eur(y.TaxEUR),
			eur(y.InitialPeriodTaxEUR) + " / " + eur(y.LaterPeriodTaxEUR),
			eur(y.LossesForwardInEUR) + " / " + eur(y.LossesForwardOutEUR),
		})
	}
	t.Render()
	fmt.Fprintln(w)
}

func consoleExitTax(w io.Writer, dt Data) {
	r := dt.ExitTax
	if r == nil || (len(r.Years) == 0 && len(r.Deemed) == 0) {
		fmt.Fprintf(w, "Exit tax (EU UCITS ETFs): no disposals.\n\n")
		return
	}
	if len(r.Years) > 0 {
		t := newTable(w, "Exit tax (EU UCITS ETFs)")
		t.AppendHeader(table.Row{"Year", "Proceeds", "Taxable gains", "Disallowed losses", "Tax", "Due"})
		rightAlign(t, 6, 6)
		for _, y := range r.Years {
			if dt.Year != 0 && y.Year != dt.Year {
				continue
			}
			t.AppendRow(table.Row{y.Year, eur(y.ProceedsEUR), eur(y.GainsEUR), neg(y.DisallowedLosses), eur(y.TaxEUR), date(y.DueDate)})
		}
		t.Render()
		fmt.Fprintln(w)
	}
	if len(r.Deemed) == 0 {
		return
	}
	var past []exittax.Deemed
	for _, d := range r.Deemed {
		if d.Past {
			past = append(past, d)
		}
	}
	if len(past) > 0 {
		fmt.Fprintf(w, "%d deemed disposal(s) already due. Report each with the fund's price on that date:\n", len(past))
		t := newTable(w, "")
		t.AppendHeader(table.Row{"Date", "Fund", "Bought", "Qty", "Cost", "Rate"})
		rightAlign(t, 6, 2, 3)
		for _, d := range past {
			t.AppendRow(table.Row{date(d.Date), dt.name(d.Lot.Key), date(d.Lot.AcquiredAt), d.Qty.String(), eur(d.CostEUR), pct(d.Rate)})
		}
		t.Render()
		fmt.Fprintln(w)
	}
	if n := len(r.Deemed) - len(past); n > 0 {
		next := r.Deemed[len(past)]
		fmt.Fprintf(w, "Next deemed disposal: %s (%s, %s lot of %s). %d upcoming in total.\n",
			date(next.Date), dt.name(next.Lot.Key), date(next.Lot.AcquiredAt), eur(next.CostEUR), n)
	}
}
