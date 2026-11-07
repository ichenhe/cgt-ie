package exittax

import (
	"testing"
	"time"

	"github.com/shopspring/decimal"

	"github.com/ichenhe/cgt-ie/internal/config"
	"github.com/ichenhe/cgt-ie/internal/model"
)

func d(s string) decimal.Decimal { return decimal.RequireFromString(s) }

func day(s string) time.Time {
	t, err := time.Parse("2006-01-02", s)
	if err != nil {
		panic(err)
	}
	return t.Add(12 * time.Hour)
}

func buy(id, when, qty, total string) model.Trade {
	return model.Trade{ID: id, ISIN: "IE00B5BMR087", Time: day(when), Kind: model.Buy, Qty: d(qty), TotalEUR: d(total)}
}

func sell(id, when, qty, total string) model.Trade {
	return model.Trade{ID: id, ISIN: "IE00B5BMR087", Time: day(when), Kind: model.Sell, Qty: d(qty), TotalEUR: d(total)}
}

var rules = config.ExitTaxRules{
	DeemedDisposalYrs: 8,
	Rates: []config.RateBand{
		{From: day("2014-01-01"), Rate: d("0.41")},
		{From: day("2026-01-01"), Rate: d("0.38")},
		{From: day("2027-01-01"), Rate: d("0.35")},
	},
}

func eq(t *testing.T, name string, got, want decimal.Decimal) {
	t.Helper()
	if !got.Equal(want) {
		t.Errorf("%s = %s, want %s", name, got, want)
	}
}

func TestFIFOGainTaxedAtDateRate(t *testing.T) {
	res, err := Compute(Input{Trades: []model.Trade{
		buy("b1", "2024-01-10", "10", "1000"),
		buy("b2", "2024-01-20", "10", "1200"), // within 4 weeks of the sale, but FIFO still applies
		sell("s1", "2024-02-01", "10", "1500"),
		sell("s2", "2026-03-01", "10", "1500"),
	}, Rules: rules, Now: day("2026-11-01")})
	if err != nil {
		t.Fatal(err)
	}
	s1, s2 := res.Disposals[0], res.Disposals[1]
	if s1.Matches[0].Lot.TradeID != "b1" {
		t.Errorf("s1 matched %s, want b1 (FIFO)", s1.Matches[0].Lot.TradeID)
	}
	eq(t, "s1 gain", s1.GainEUR, d("500"))
	eq(t, "s1 rate", s1.Rate, d("0.41"))
	eq(t, "s1 tax", s1.TaxEUR, d("205.00"))
	eq(t, "s2 gain", s2.GainEUR, d("300"))
	eq(t, "s2 rate", s2.Rate, d("0.38"))
	eq(t, "s2 tax", s2.TaxEUR, d("114.00"))

	if len(res.Years) != 2 || res.Years[0].Year != 2024 || res.Years[1].Year != 2026 {
		t.Fatalf("years = %+v", res.Years)
	}
	eq(t, "2024 tax", res.Years[0].TaxEUR, d("205.00"))
	if res.Years[0].DueDate.Format("2006-01-02") != "2025-10-31" {
		t.Errorf("due = %s", res.Years[0].DueDate)
	}
}

func TestLossDisallowedAndNotNetted(t *testing.T) {
	res, err := Compute(Input{Trades: []model.Trade{
		buy("b1", "2024-01-10", "10", "1000"),
		buy("b2", "2024-01-11", "10", "1000"),
		sell("s1", "2024-06-01", "10", "800"),  // loss 200
		sell("s2", "2024-07-01", "10", "1100"), // gain 100
	}, Rules: rules, Now: day("2026-11-01")})
	if err != nil {
		t.Fatal(err)
	}
	eq(t, "s1 taxable", res.Disposals[0].TaxableEUR, d("0"))
	eq(t, "s1 tax", res.Disposals[0].TaxEUR, d("0"))
	ys := res.Years[0]
	eq(t, "gains", ys.GainsEUR, d("100"))
	eq(t, "disallowed", ys.DisallowedLosses, d("200"))
	eq(t, "tax", ys.TaxEUR, d("41.00")) // the loss does not reduce it
}

func TestDeemedDisposalSchedule(t *testing.T) {
	res, err := Compute(Input{Trades: []model.Trade{
		buy("b1", "2017-03-15", "10", "1000"),
		buy("b2", "2019-06-30", "5", "600"),
		buy("b3", "2024-01-10", "5", "700"),
		sell("s1", "2020-01-01", "4", "500"), // consumes part of b1
	}, Rules: rules, Now: day("2026-11-01")})
	if err != nil {
		t.Fatal(err)
	}
	if len(res.Deemed) != 3 {
		t.Fatalf("deemed = %+v", res.Deemed)
	}
	d1, d2, d3 := res.Deemed[0], res.Deemed[1], res.Deemed[2]

	if d1.Date.Format("2006-01-02") != "2025-03-15" || !d1.Past {
		t.Errorf("d1 = %+v", d1)
	}
	eq(t, "d1 qty", d1.Qty, d("6"))
	eq(t, "d1 cost", d1.CostEUR, d("600"))
	eq(t, "d1 rate", d1.Rate, d("0.41"))

	if d2.Date.Format("2006-01-02") != "2027-06-30" || d2.Past {
		t.Errorf("d2 = %+v", d2)
	}
	eq(t, "d2 rate", d2.Rate, d("0.35"))

	if d3.Date.Format("2006-01-02") != "2032-01-10" {
		t.Errorf("d3 = %+v", d3)
	}
	if len(res.Warnings) != 1 {
		t.Errorf("warnings = %v", res.Warnings)
	}
}
