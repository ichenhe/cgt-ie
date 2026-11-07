package fx

import (
	"strings"
	"testing"
	"time"

	"github.com/shopspring/decimal"
)

const sample = `Date,USD,JPY,GBP,
2026-03-02,1.1600,160.10,0.8400,
2026-02-27,1.1550,159.80,0.8390,
2026-02-26,1.1500,N/A,0.8380,
`

func day(s string) time.Time {
	t, _ := time.Parse("2006-01-02", s)
	return t
}

func TestRateExactAndFallback(t *testing.T) {
	r, err := Parse(strings.NewReader(sample))
	if err != nil {
		t.Fatal(err)
	}
	lk, err := r.Rate("USD", day("2026-02-26"))
	if err != nil || !lk.Rate.Equal(decimal.RequireFromString("1.15")) || lk.Fallback {
		t.Errorf("exact: %+v %v", lk, err)
	}
	// Saturday 28 Feb → Friday 27 Feb
	lk, err = r.Rate("usd", day("2026-02-28"))
	if err != nil || !lk.Rate.Equal(decimal.RequireFromString("1.155")) || !lk.Fallback || lk.Date != day("2026-02-27") {
		t.Errorf("fallback: %+v %v", lk, err)
	}
	// JPY N/A on the 26th → no earlier value in the sample → error
	if _, err := r.Rate("JPY", day("2026-02-26")); err == nil {
		t.Error("expected error for N/A with no earlier date")
	}
	if _, err := r.Rate("CHF", day("2026-03-02")); err == nil {
		t.Error("expected error for unknown currency")
	}
	if _, err := r.Rate("USD", day("2026-03-15")); err == nil {
		t.Error("expected error beyond fallback window")
	}
	if r.Latest() != day("2026-03-02") {
		t.Errorf("latest = %s", r.Latest())
	}
}

func TestToEUR(t *testing.T) {
	r, _ := Parse(strings.NewReader(sample))
	eur, lk, err := r.ToEUR(decimal.NewFromInt(115), "USD", day("2026-02-26"))
	if err != nil {
		t.Fatal(err)
	}
	if !eur.Equal(decimal.NewFromInt(100)) {
		t.Errorf("115 USD @1.15 = %s EUR, want 100", eur)
	}
	if lk.Fallback {
		t.Error("unexpected fallback")
	}
	eur, _, _ = r.ToEUR(decimal.NewFromInt(1), "EUR", day("2026-02-26"))
	if !eur.Equal(decimal.NewFromInt(1)) {
		t.Errorf("EUR passthrough = %s", eur)
	}
}

func TestRejectsNonECBFile(t *testing.T) {
	if _, err := Parse(strings.NewReader("Action,Time\nx,y\n")); err == nil {
		t.Error("expected error")
	}
}
