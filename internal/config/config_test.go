package config

import (
	"testing"
	"time"

	"github.com/shopspring/decimal"
)

func TestLoadRulesAndRateOn(t *testing.T) {
	r, err := LoadRules("../../config/rules.yaml")
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

func TestLoadInstruments(t *testing.T) {
	ins, err := LoadInstruments("../../config/instruments.yaml")
	if err != nil {
		t.Fatal(err)
	}
	if ins["IE00B5BMR087"].Regime != "exit_tax" {
		t.Errorf("CSPX regime = %q", ins["IE00B5BMR087"].Regime)
	}
	missing, err := LoadInstruments("does-not-exist.yaml")
	if err != nil || len(missing) != 0 {
		t.Errorf("missing file should yield empty map, got %v %v", missing, err)
	}
}
