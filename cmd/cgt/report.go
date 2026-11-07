package main

import (
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/shopspring/decimal"

	"github.com/ichenhe/cgt-ie/internal/broker"
	"github.com/ichenhe/cgt-ie/internal/classify"
	"github.com/ichenhe/cgt-ie/internal/config"
	"github.com/ichenhe/cgt-ie/internal/model"
	"github.com/ichenhe/cgt-ie/internal/report"
	"github.com/ichenhe/cgt-ie/internal/tax/cgt"
	"github.com/ichenhe/cgt-ie/internal/tax/exittax"
)

type reportCmd struct {
	configFlags
	Year              int       `help:"Only report this tax year (0 = all years)." default:"0"`
	OutputDir         string    `short:"o" help:"Directory for report files." default:"cgt-reports" type:"path"`
	NoLossRestriction bool      `name:"no-loss-restriction" help:"Disable the s.581(3) four-week loss restriction."`
	LossesForward     eurAmount `help:"CGT losses carried forward from before (EUR)." default:"0"`
	Today             *dateFlag `help:"Treat this date as today for deemed-disposal status (default: today)." placeholder:"YYYY-MM-DD"`
	FX                string    `help:"ECB eurofxref-hist.csv used for trades settled in non-EUR cash; downloaded or refreshed automatically when needed." default:"${default_fx_path}" type:"path"`
	TxFiles           []string  `arg:"" name:"export.csv" help:"Trading 212 transaction export(s), CSV." type:"existingfile"`
}

// eurAmount parses a non-negative decimal flag.
type eurAmount struct{ decimal.Decimal }

func (a *eurAmount) UnmarshalText(b []byte) error {
	d, err := decimal.NewFromString(string(b))
	if err != nil || d.Sign() < 0 {
		return fmt.Errorf("must be a non-negative number, got %q", b)
	}
	a.Decimal = d
	return nil
}

// dateFlag parses a YYYY-MM-DD flag as a UTC date.
type dateFlag struct{ time.Time }

func (d *dateFlag) UnmarshalText(b []byte) error {
	t, err := time.Parse("2006-01-02", string(b))
	if err != nil {
		return fmt.Errorf("want YYYY-MM-DD, got %q", b)
	}
	d.Time = t
	return nil
}

func (c *reportCmd) Run() error {
	rules, err := config.LoadRules(c.rulesPath())
	if err != nil {
		return err
	}
	instrPath := c.instrumentsPath()
	instrs, err := config.LoadInstruments(instrPath)
	if err != nil {
		return err
	}
	now := time.Now().UTC()
	if c.Today != nil {
		now = c.Today.Time
	}

	parsed, err := loadTrades(c.TxFiles)
	if err != nil {
		return err
	}
	warnings := append([]string{}, parsed.Warnings...)
	if parsed.Duplicates > 0 {
		warnings = append(warnings, fmt.Sprintf("%d duplicate trade rows ignored (overlapping exports)", parsed.Duplicates))
	}
	if n := len(parsed.Skipped); n > 0 {
		warnings = append(warnings, fmt.Sprintf("%d cash-only rows skipped (deposits, interest, card, currency conversion, ...)", n))
	}

	byKind := map[model.Kind]int{}
	var trades []model.Trade
	for _, t := range parsed.Trades {
		if t.Kind.IsTrade() {
			trades = append(trades, t)
		} else {
			byKind[t.Kind]++
		}
	}
	if n := byKind[model.Dividend]; n > 0 {
		warnings = append(warnings, fmt.Sprintf("%d dividend rows not processed: share dividends are income tax; distributions from EU funds are taxed at the exit tax rate. Report them separately.", n))
	}
	if n := byKind[model.Split]; n > 0 {
		warnings = append(warnings, fmt.Sprintf("%d stock split rows not processed; quantities before the split are NOT adjusted. Check affected holdings by hand.", n))
	}
	if n := byKind[model.TransferIn] + byKind[model.TransferOut]; n > 0 {
		warnings = append(warnings, fmt.Sprintf("%d share transfer rows not processed; lots moved between brokers are not tracked.", n))
	}
	parsed.Trades = trades

	var conv broker.Converter
	rates, err := loadRatesFor(parsed.Trades, c.FX)
	if err != nil {
		return err
	} else if rates != nil {
		conv = rates
	}
	fxUsed := false
	if n, err := broker.ConvertToEUR(parsed.Trades, conv); err != nil {
		return err
	} else if n > 0 {
		fxUsed = true
		warnings = append(warnings, fmt.Sprintf(
			"%d trade(s) were settled in non-EUR cash; cost/proceeds converted at the ECB rate on the trade date. "+
				"Note: holding foreign currency is itself a CGT asset; gains/losses on the cash balance are not computed here.", n))
	}

	// abort on unclassified instruments
	if un := classify.Unclassified(parsed.Trades, instrs); len(un) > 0 {
		var b strings.Builder
		fmt.Fprintf(&b, "%d instrument(s) are not in %s:\n", len(un), instrPath)
		for _, t := range un {
			g := classify.Guess(t.Key(), t.ISIN, t.Ticker, t.Name)
			fmt.Fprintf(&b, "  %-8s %-13s %-40s suggested: %s\n", t.Ticker, t.Key(), trunc(t.Name, 40), g.Instrument.Regime)
		}
		fmt.Fprintf(&b, "run 'cgt classify -o %s.suggested.yaml <exports>' and review it", strings.TrimSuffix(instrPath, ".yaml"))
		return fmt.Errorf("%s", b.String())
	}

	var cgtTrades, etfTrades []model.Trade
	usFunds := map[string]struct{}{}
	for _, t := range parsed.Trades {
		switch instrs[t.Key()].Regime {
		case model.RegimeCGT:
			cgtTrades = append(cgtTrades, t)
		case model.RegimeExitTax:
			etfTrades = append(etfTrades, t)
			if isin := instrs[t.Key()].ISIN; strings.HasPrefix(strings.ToUpper(isin), "US") {
				usFunds[t.Ticker] = struct{}{}
			}
		}
	}
	if len(usFunds) > 0 {
		tickers := make([]string, 0, len(usFunds))
		for k := range usFunds {
			tickers = append(tickers, k)
		}
		sort.Strings(tickers)
		warnings = append(warnings, fmt.Sprintf(
			"US-domiciled fund(s) %s are taxed here as exit tax. Revenue has given no confirmation either way since 1 Jan 2022 (TDM 27-01A-03): "+
				"exit tax applies only if the fund is 'similar in all material respects' to an Irish fund (s.747B(2A)); otherwise CGT. "+
				"For units held before 2022 the 8-year deemed-disposal clock runs from 2022 (earliest 2030), which this tool does not adjust.",
			strings.Join(tickers, ", ")))
	}

	cgtRes, err := cgt.Compute(cgt.Input{
		Trades:               cgtTrades,
		Rules:                rules.CGT,
		ApplyLossRestriction: !c.NoLossRestriction,
		LossesForwardIn:      c.LossesForward.Decimal,
	})
	if err != nil {
		return fmt.Errorf("cgt: %w", err)
	}
	etfRes, err := exittax.Compute(exittax.Input{Trades: etfTrades, Rules: rules.ExitTax, Now: now})
	if err != nil {
		return fmt.Errorf("exit tax: %w", err)
	}
	warnings = append(warnings, cgtRes.Warnings...)
	warnings = append(warnings, etfRes.Warnings...)
	warnings = append(warnings, crossCheck(cgtRes, etfRes)...)

	data := report.Data{
		Year:                   c.Year,
		CGT:                    cgtRes,
		ExitTax:                etfRes,
		Instruments:            instrs,
		Warnings:               warnings,
		GeneratedAt:            now,
		LossRestrictionApplied: !c.NoLossRestriction,
		Rules:                  rules,
		SourceFiles:            c.TxFiles,
	}

	if err := os.MkdirAll(c.OutputDir, 0o755); err != nil {
		return err
	}
	// File names carry the tax year(s) covered: "-2026" or "-2025-2026".
	suffix := ""
	if first, last, ok := data.YearSpan(); ok {
		suffix = fmt.Sprintf("-%d", first)
		if last != first {
			suffix += fmt.Sprintf("-%d", last)
		}
	}
	mdPath := filepath.Join(c.OutputDir, "report"+suffix+".md")
	csvPath := filepath.Join(c.OutputDir, "sales-breakdown"+suffix+".csv")

	mdf, err := os.Create(mdPath)
	if err != nil {
		return err
	}
	defer mdf.Close()
	report.Markdown(mdf, data)

	csvf, err := os.Create(csvPath)
	if err != nil {
		return err
	}
	defer csvf.Close()
	if err := report.SalesBreakdownCSV(csvf, data); err != nil {
		return err
	}

	generated := []string{mdPath, filepath.Base(csvPath)}

	// Keep the rate table that produced the figures next to the report, named
	// by the latest rate date it contains, so the computation can be
	// reproduced later even after fx-update has replaced the file.
	if fxUsed {
		fxCopy := filepath.Join(c.OutputDir, "ecb-fxrate-"+rates.Latest().Format("20060102")+".csv")
		if err := copyFile(c.FX, fxCopy); err != nil {
			return fmt.Errorf("copy FX rates: %w", err)
		}
		generated = append(generated, filepath.Base(fxCopy))
	}

	report.Console(os.Stdout, data)
	fmt.Fprintf(os.Stderr, "Generated: %s\n", strings.Join(generated, ", "))
	return nil
}

func copyFile(src, dst string) error {
	in, err := os.Open(src)
	if err != nil {
		return err
	}
	defer in.Close()
	out, err := os.Create(dst)
	if err != nil {
		return err
	}
	if _, err := io.Copy(out, in); err != nil {
		out.Close()
		return err
	}
	return out.Close()
}

// crossCheck compares our per-disposal gain with Trading 212's own "Result"
// column. Trading 212 uses average cost and ignores all fees, so small
// differences are normal and larger ones are expected whenever the 4-week
// LIFO rule bites. A gap of the same order as the proceeds usually means
// earlier buys are missing from the exports.
func crossCheck(c *cgt.Result, e *exittax.Result) []string {
	var large []string
	small, maxSmall := 0, decimal.Zero
	tolerance := decimal.RequireFromString("0.05") // ignore diff <= €0.05 - mainly due to precision
	check := func(ticker, day string, ours, theirs, proceeds decimal.Decimal) {
		diff := ours.Sub(theirs).Abs()
		if !diff.GreaterThan(tolerance) {
			return
		}
		if proceeds.Sign() > 0 && diff.GreaterThan(proceeds.Mul(decimal.RequireFromString("0.25"))) {
			large = append(large, fmt.Sprintf("%s %s: gain %s vs Trading 212 'Result' %s — LARGE difference, check that all earlier buys are in the exports",
				day, ticker, ours.StringFixed(2), theirs.StringFixed(2)))
			return
		}
		small++
		if diff.GreaterThan(maxSmall) {
			maxSmall = diff
		}
	}
	for _, d := range c.Disposals {
		if d.Trade.HasBrokerResult {
			check(d.Trade.Ticker, d.Trade.Day().Format("2006-01-02"), d.RawGainEUR, d.Trade.BrokerResult, d.ProceedsEUR)
		}
	}
	for _, d := range e.Disposals {
		if d.Trade.HasBrokerResult {
			check(d.Trade.Ticker, d.Trade.Day().Format("2006-01-02"), d.GainEUR, d.Trade.BrokerResult, d.ProceedsEUR)
		}
	}
	out := large
	if small > 0 {
		out = append(out, fmt.Sprintf("%d disposal(s) differ from Trading 212's 'Result' column by up to €%s; expected, because Trading 212 uses average cost and this tool uses FIFO / 4-week LIFO as Irish law requires",
			small, maxSmall.StringFixed(2)))
	}
	return out
}

func trunc(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n-1] + "…"
}
