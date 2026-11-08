package config

import (
	"os"
	"testing"
	"time"

	"github.com/shopspring/decimal"

	"github.com/ichenhe/cgt-ie/internal/model"
)

func TestLoadRulesAndRateOn(t *testing.T) {
	r, err := LoadRules("")
	if err != nil {
		t.Fatal(err)
	}
	for _, c := range []struct{ d, want string }{
		{"2026-10-06", "0.33"},
		{"2026-10-07", "0.31"},
	} {
		d, _ := time.Parse("2006-01-02", c.d)
		got, err := r.CGT.RateOn(d)
		if err != nil || !got.Equal(decimal.RequireFromString(c.want)) {
			t.Errorf("CGT RateOn(%s) = %s %v, want %s", c.d, got, err, c.want)
		}
	}
	if !r.CGT.AnnualExemption.Equal(decimal.NewFromInt(1270)) {
		t.Errorf("exemption = %s", r.CGT.AnnualExemption)
	}
	if r.CGT.MatchingWindow != 28 || r.CGT.ReacquisitionWindow != 28 {
		t.Errorf("windows = %d %d", r.CGT.MatchingWindow, r.CGT.ReacquisitionWindow)
	}
	cases := []struct {
		d    string
		want string
	}{
		{"2025-12-31", "0.41"},
		{"2026-01-01", "0.38"},
		{"2026-06-15", "0.38"},
		{"2027-01-01", "0.35"},
	}
	for _, c := range cases {
		d, _ := time.Parse("2006-01-02", c.d)
		got, err := r.ExitTax.RateOn(d)
		if err != nil {
			t.Fatal(err)
		}
		if !got.Equal(decimal.RequireFromString(c.want)) {
			t.Errorf("RateOn(%s) = %s, want %s", c.d, got, c.want)
		}
	}
	if _, err := r.ExitTax.RateOn(time.Date(2000, 1, 1, 0, 0, 0, 0, time.UTC)); err == nil {
		t.Error("expected error for date before first band")
	}
}

// A user file sets only what it wants to change; everything else stays default.
func TestLoadRulesPartialOverride(t *testing.T) {
	path := t.TempDir() + "/rules.yaml"
	os.WriteFile(path, []byte("cgt:\n  annual_exemption: 2000.1234567890123456789\nexit_tax:\n  rates:\n    - from: 2020-01-01\n      rate: 0.5\n"), 0o644)
	r, err := LoadRules(path)
	if err != nil {
		t.Fatal(err)
	}
	// Decimals are read from their text: no float64 on the way.
	if r.CGT.AnnualExemption.String() != "2000.1234567890123456789" {
		t.Errorf("exemption = %s", r.CGT.AnnualExemption)
	}
	if len(r.CGT.Rates) != 2 || r.CGT.MatchingWindow != 28 { // defaults kept
		t.Errorf("cgt defaults lost: %+v", r.CGT)
	}
	if len(r.ExitTax.Rates) != 1 || !r.ExitTax.Rates[0].Rate.Equal(decimal.RequireFromString("0.5")) { // list replaced whole
		t.Errorf("exit rates = %+v", r.ExitTax.Rates)
	}
	if r.ExitTax.DeemedDisposalYrs != 8 {
		t.Errorf("deemed years = %d", r.ExitTax.DeemedDisposalYrs)
	}
	if _, err := LoadRules(t.TempDir() + "/missing.yaml"); err == nil {
		t.Error("explicit missing file should error")
	}
}

func TestLoadInstruments(t *testing.T) {
	dir := t.TempDir()
	path := dir + "/instruments.yaml"
	if err := os.WriteFile(path, []byte(`
instruments:
  - isin: ie00b5bmr087
    ticker: CSPX
    regime: exit_tax
  - ticker: AAPL
    regime: cgt
  - ticker: VUSA
    broker: revolut
    regime: exit_tax
`), 0o644); err != nil {
		t.Fatal(err)
	}

	o, err := LoadInstruments(path)
	if err != nil {
		t.Fatal(err)
	}
	if len(o) != 3 {
		t.Fatalf("len = %d", len(o))
	}
	trade := func(isin, broker, ticker string) model.Trade {
		return model.Trade{ISIN: isin, Ticker: ticker, Source: model.Source{Broker: broker}}
	}
	// By ISIN, case-insensitive.
	if in, ok := o.Lookup(trade("IE00B5BMR087", "t212", "CSPX")); !ok || in.Regime != "exit_tax" {
		t.Errorf("CSPX: %+v %v", in, ok)
	}
	// By ticker, any broker; identifiers come from the trade.
	if in, ok := o.Lookup(trade("US0378331005", "t212", "AAPL")); !ok || in.Regime != "cgt" || in.Key() != "US0378331005" {
		t.Errorf("AAPL: %+v %v", in, ok)
	}
	if in, ok := o.Lookup(trade("", "freetrade", "aapl")); !ok || in.Key() != "freetrade:aapl" {
		t.Errorf("AAPL no-ISIN: %+v %v", in, ok)
	}
	// By ticker restricted to a broker.
	if _, ok := o.Lookup(trade("", "revolut", "VUSA")); !ok {
		t.Error("VUSA via revolut should match")
	}
	if _, ok := o.Lookup(trade("", "t212", "VUSA")); ok {
		t.Error("VUSA via t212 should not match")
	}
	if _, ok := o.Lookup(trade("US9999999999", "t212", "ZZZ")); ok {
		t.Error("unknown should not match")
	}
	// nil Overrides is a valid "no overrides".
	if _, ok := Overrides(nil).Lookup(trade("US0378331005", "t212", "AAPL")); ok {
		t.Error("nil overrides matched")
	}

	if _, err := LoadInstruments(dir + "/missing.yaml"); err == nil {
		t.Error("missing file should be an error")
	}
	bad := dir + "/bad.yaml"
	os.WriteFile(bad, []byte("instruments:\n  - regime: cgt\n"), 0o644)
	if _, err := LoadInstruments(bad); err == nil {
		t.Error("entry without isin or ticker should fail")
	}
}
