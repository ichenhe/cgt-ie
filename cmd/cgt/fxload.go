package main

import (
	"fmt"
	"os"

	"github.com/ichenhe/cgt-ie/internal/fx"
	"github.com/ichenhe/cgt-ie/internal/model"
)

// loadRatesFor returns ECB rates that can convert every non-EUR trade,
// downloading or refreshing the file at path when needed. It returns nil
// rates (and nil error) when no trade needs conversion.
func loadRatesFor(trades []model.Trade, path string) (*fx.Rates, error) {
	var needed []model.Trade
	for _, t := range trades {
		if t.Kind.IsTrade() && t.NeedsFX() {
			needed = append(needed, t)
		}
	}
	if len(needed) == 0 {
		return nil, nil
	}

	rates, err := fx.Load(path)
	switch {
	case err == nil && covers(rates, needed):
		return rates, nil
	case err == nil:
		fmt.Fprintf(os.Stderr, "ECB rates in %s end on %s, before the latest non-EUR trade; updating ...\n",
			path, rates.Latest().Format("2006-01-02"))
	case os.IsNotExist(err):
		fmt.Fprintf(os.Stderr, "%d trade(s) settled in non-EUR cash need ECB rates; downloading to %s ...\n", len(needed), path)
	default:
		return nil, fmt.Errorf("--fx %s: %w", path, err)
	}

	if err := fx.Download(path); err != nil {
		return nil, fmt.Errorf("download ECB rates: %w (run 'cgt fx-update' when online, or pass --fx)", err)
	}
	rates, err = fx.Load(path)
	if err != nil {
		return nil, fmt.Errorf("--fx %s: %w", path, err)
	}
	if !covers(rates, needed) {
		// Downloaded and still can not cover: typically the ECB has not yet
		// published today's rate.
		// Return what we have and let's compute engine to decide what to do.
		fmt.Fprintf(os.Stderr, "warning: ECB rates end on %s and may not cover the latest trades\n", rates.Latest().Format("2006-01-02"))
	}
	return rates, nil
}

// covers reports whether rates can convert every non-EUR amount in trades.
func covers(rates *fx.Rates, trades []model.Trade) bool {
	for _, t := range trades {
		for _, m := range append([]model.Money{t.Gross}, t.Fees...) {
			if m.IsEUR() {
				continue
			}
			if _, err := rates.Rate(m.CCY, t.Day()); err != nil {
				return false
			}
		}
	}
	return true
}
