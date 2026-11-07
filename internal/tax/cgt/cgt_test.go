package cgt

import (
	"testing"
	"time"

	"github.com/shopspring/decimal"

	"github.com/ichenhe/cgt-ie/internal/config"
	"github.com/ichenhe/cgt-ie/internal/model"
)

func d(s string) decimal.Decimal { return decimal.RequireFromString(s) }

func at(s string) time.Time {
	t, err := time.Parse("2006-01-02 15:04", s)
	if err != nil {
		t2, err2 := time.Parse("2006-01-02", s)
		if err2 != nil {
			panic(err)
		}
		return t2.Add(12 * time.Hour)
	}
	return t
}

func buy(isin, id, when, qty, total string) model.Trade {
	return model.Trade{ID: id, ISIN: isin, Time: at(when), Kind: model.Buy, Qty: d(qty), TotalEUR: d(total)}
}

func sell(isin, id, when, qty, total string) model.Trade {
	return model.Trade{ID: id, ISIN: isin, Time: at(when), Kind: model.Sell, Qty: d(qty), TotalEUR: d(total)}
}

var rules = config.CGTRules{
	Rates: []config.RateBand{
		{From: at("2012-12-06"), Rate: d("0.33")},
		{From: at("2026-10-07"), Rate: d("0.31")},
	},
	AnnualExemption:     d("1270"),
	MatchingWindow:      28,
	ReacquisitionWindow: 28,
}

func run(t *testing.T, trades []model.Trade, restrict bool) *Result {
	t.Helper()
	res, err := Compute(Input{Trades: trades, Rules: rules, ApplyLossRestriction: restrict})
	if err != nil {
		t.Fatal(err)
	}
	return res
}

func eq(t *testing.T, name string, got, want decimal.Decimal) {
	t.Helper()
	if !got.Equal(want) {
		t.Errorf("%s = %s, want %s", name, got, want)
	}
}

// Revenue's Jane example:: buy 3000 for €3000 on 1 Apr, sell 14 Apr for €2000, no
// reacquisition. Under s.581(3) the loss is allowable in full.
func TestLossWithoutReacquisitionIsAllowable(t *testing.T) {
	res := run(t, []model.Trade{
		buy("ABC", "b", "2017-04-01", "3000", "3000"),
		sell("ABC", "s", "2017-04-14", "3000", "2000"),
	}, true)
	disp := res.Disposals[0]
	eq(t, "allowable", disp.AllowableLossEUR, d("1000"))
	eq(t, "restricted", disp.RestrictedLossEUR, d("0"))
	ys := res.Years[0]
	eq(t, "forward out", ys.LossesForwardOutEUR, d("1000"))
	eq(t, "tax", ys.TaxEUR, d("0"))
}

// Revenue's Kevin example: Jane case + buys 3000 more on 21 Apr for €2500 and
// sells them next year for €4000. Loss of €1000 restricted; deducted from the
// €1500 gain on the reacquired shares → chargeable gain €500.
func TestLossRestrictedAndReleasedOnReacquiredShares(t *testing.T) {
	res := run(t, []model.Trade{
		buy("ABC", "b1", "2017-04-01", "3000", "3000"),
		sell("ABC", "s1", "2017-04-14", "3000", "2000"),
		buy("ABC", "b2", "2017-04-21", "3000", "2500"),
		sell("ABC", "s2", "2018-03-01", "3000", "4000"),
	}, true)

	s1, s2 := res.Disposals[0], res.Disposals[1]
	eq(t, "s1 allowable", s1.AllowableLossEUR, d("0"))
	eq(t, "s1 restricted", s1.RestrictedLossEUR, d("1000"))
	eq(t, "s1 reacquired", s1.ReacquiredQty, d("3000"))

	eq(t, "s2 raw gain", s2.RawGainEUR, d("1500"))
	eq(t, "s2 released used", s2.ReleasedLossUsedEUR, d("1000"))
	eq(t, "s2 chargeable", s2.ChargeableGainEUR, d("500"))

	y2017, y2018 := res.Years[0], res.Years[1]
	eq(t, "2017 allowable losses", y2017.AllowableLossesEUR, d("0"))
	eq(t, "2017 restricted", y2017.RestrictedLossesEUR, d("1000"))
	eq(t, "2017 forward out", y2017.LossesForwardOutEUR, d("0"))
	eq(t, "2018 gains", y2018.ChargeableGainsEUR, d("500"))
	eq(t, "2018 exemption used", y2018.ExemptionUsedEUR, d("500"))
	eq(t, "2018 tax", y2018.TaxEUR, d("0"))
}

// Partial reacquisition: sell 100 at a €100 loss, rebuy 40 within 4 weeks.
// 60% (€60) allowable now; €40 restricted and attached to the 40 shares.
// Selling those 40 later at a €50 gain gives chargeable gain €10.
func TestPartialReacquisitionSplitsLoss(t *testing.T) {
	res := run(t, []model.Trade{
		buy("ABC", "b1", "2024-01-10", "100", "1000"),
		sell("ABC", "s1", "2024-03-01", "100", "900"),
		buy("ABC", "b2", "2024-03-10", "40", "360"),
		sell("ABC", "s2", "2024-06-01", "40", "410"),
	}, true)

	s1, s2 := res.Disposals[0], res.Disposals[1]
	eq(t, "s1 allowable", s1.AllowableLossEUR, d("60"))
	eq(t, "s1 restricted", s1.RestrictedLossEUR, d("40"))
	eq(t, "b2 lot restricted", res.Books["ABC"].Lots[1].RestrictedLossEUR, d("0")) // released by s2
	eq(t, "s2 raw gain", s2.RawGainEUR, d("50"))
	eq(t, "s2 released used", s2.ReleasedLossUsedEUR, d("40"))
	eq(t, "s2 chargeable", s2.ChargeableGainEUR, d("10"))

	ys := res.Years[0]
	eq(t, "gains", ys.ChargeableGainsEUR, d("10"))
	eq(t, "losses", ys.AllowableLossesEUR, d("60"))
	eq(t, "forward out", ys.LossesForwardOutEUR, d("50"))
}

// Restricted loss is forfeited if the reacquired shares are themselves sold
// at a loss (nothing to deduct it from).
func TestRestrictedLossForfeitedWhenReacquiredSoldAtLoss(t *testing.T) {
	res := run(t, []model.Trade{
		buy("ABC", "b1", "2024-01-10", "10", "100"),
		sell("ABC", "s1", "2024-03-01", "10", "80"),
		buy("ABC", "b2", "2024-03-05", "10", "80"),
		sell("ABC", "s2", "2024-06-01", "10", "70"),
	}, true)
	s2 := res.Disposals[1]
	eq(t, "s2 forfeited", s2.ReleasedLossForfeitedEUR, d("20"))
	eq(t, "s2 allowable", s2.AllowableLossEUR, d("10"))
	eq(t, "year allowable", res.Years[0].AllowableLossesEUR, d("10"))
}

// Partial sale of a reacquired lot releases a proportional share of the
// restricted loss.
func TestRestrictedLossReleasedProRata(t *testing.T) {
	res := run(t, []model.Trade{
		buy("ABC", "b1", "2024-01-10", "10", "100"),
		sell("ABC", "s1", "2024-03-01", "10", "60"), // loss 40
		buy("ABC", "b2", "2024-03-05", "10", "60"),
		sell("ABC", "s2", "2024-06-01", "5", "60"), // gain 30 on half; release 20
	}, true)
	s2 := res.Disposals[1]
	eq(t, "s2 released", s2.ReleasedLossUsedEUR, d("20"))
	eq(t, "s2 chargeable", s2.ChargeableGainEUR, d("10"))
	eq(t, "lot still carries", res.Books["ABC"].Lots[1].RestrictedLossEUR, d("20"))
}

// Reacquisition window boundary: 28 days after is inside, 29 is outside.
func TestReacquisitionWindowBoundary(t *testing.T) {
	for _, c := range []struct {
		rebuy      string
		restricted string
	}{
		{"2024-03-29", "100"}, // day 28
		{"2024-03-30", "0"},   // day 29
	} {
		res := run(t, []model.Trade{
			buy("ABC", "b1", "2024-01-10", "10", "1000"),
			sell("ABC", "s1", "2024-03-01", "10", "900"),
			buy("ABC", "b2", c.rebuy, "10", "900"),
		}, true)
		eq(t, "restricted rebuy "+c.rebuy, res.Disposals[0].RestrictedLossEUR, d(c.restricted))
	}
}

// Same-day: a buy later the same day counts as a reacquisition; a buy earlier
// the same day is an acquisition matched by LIFO, not a reacquisition.
func TestSameDayOrdering(t *testing.T) {
	res := run(t, []model.Trade{
		buy("ABC", "b1", "2024-01-10", "10", "1000"),
		sell("ABC", "s1", "2024-03-01 10:00", "10", "900"),
		buy("ABC", "b2", "2024-03-01 15:00", "10", "900"),
	}, true)
	eq(t, "restricted", res.Disposals[0].RestrictedLossEUR, d("100"))
}

func TestDisableRestrictedLoss(t *testing.T) {
	res := run(t, []model.Trade{
		buy("ABC", "b1", "2024-01-10", "10", "1000"),
		sell("ABC", "s1", "2024-03-01", "10", "900"),
		buy("ABC", "b2", "2024-03-05", "10", "900"),
	}, false)
	eq(t, "allowable", res.Disposals[0].AllowableLossEUR, d("100"))
	eq(t, "restricted", res.Disposals[0].RestrictedLossEUR, d("0"))
}

func TestYearSummaryExemptionRateCarryForward(t *testing.T) {
	res := run(t, []model.Trade{
		buy("A", "a1", "2023-01-05", "10", "1000"),
		sell("A", "a2", "2023-06-01", "10", "400"), // loss 600 in 2023
		buy("B", "b1", "2024-01-05", "10", "1000"),
		sell("B", "b2", "2024-06-01", "10", "3000"), // gain 2000 in 2024
	}, true)
	y23, y24 := res.Years[0], res.Years[1]
	eq(t, "2023 forward out", y23.LossesForwardOutEUR, d("600"))
	eq(t, "2023 tax", y23.TaxEUR, d("0"))
	eq(t, "2024 forward in", y24.LossesForwardInEUR, d("600"))
	eq(t, "2024 net", y24.NetGainEUR, d("1400"))
	eq(t, "2024 exemption", y24.ExemptionUsedEUR, d("1270"))
	eq(t, "2024 taxable", y24.TaxableEUR, d("130"))
	eq(t, "2024 tax", y24.TaxEUR, d("42.90"))
	eq(t, "2024 forward out", y24.LossesForwardOutEUR, d("0"))
}

func TestYearWithNoDisposalsStillCarriesLossForward(t *testing.T) {
	res := run(t, []model.Trade{
		buy("A", "a1", "2022-01-05", "10", "1000"),
		sell("A", "a2", "2022-06-01", "10", "400"),
		buy("B", "b1", "2024-01-05", "10", "1000"),
		sell("B", "b2", "2024-06-01", "10", "1500"),
	}, true)
	if len(res.Years) != 3 {
		t.Fatalf("years = %d", len(res.Years))
	}
	eq(t, "2023 forward", res.Years[1].LossesForwardOutEUR, d("600"))
	eq(t, "2024 net", res.Years[2].NetGainEUR, d("0"))
	eq(t, "2024 forward", res.Years[2].LossesForwardOutEUR, d("100"))
}

func TestLossesForwardInput(t *testing.T) {
	res, err := Compute(Input{
		Trades: []model.Trade{
			buy("A", "a1", "2024-01-05", "10", "1000"),
			sell("A", "a2", "2024-06-01", "10", "2000"),
		},
		Rules:           rules,
		LossesForwardIn: d("500"),
	})
	if err != nil {
		t.Fatal(err)
	}
	eq(t, "net", res.Years[0].NetGainEUR, d("500"))
}

func TestDecemberPaymentSplit(t *testing.T) {
	res := run(t, []model.Trade{
		buy("A", "a1", "2024-01-05", "20", "2000"),
		sell("A", "a2", "2024-06-01", "10", "3000"), // gain 2000, Jan–Nov
		sell("A", "a3", "2024-12-10", "10", "2000"), // gain 1000, Dec
	}, true)
	ys := res.Years[0]
	eq(t, "tax", ys.TaxEUR, d("570.90"))                         // (3000-1270)*0.33
	eq(t, "initial period", ys.InitialPeriodTaxEUR, d("240.90")) // (2000-1270)*0.33
	eq(t, "later period", ys.LaterPeriodTaxEUR, d("330.00"))     // remainder
}

// 2026: 33% up to 6 Oct, 31% from 7 Oct. Exemption and losses are allocated
// to gains in date order, so they reduce the 33% gain first.
func TestRateChangeMidYear(t *testing.T) {
	res := run(t, []model.Trade{
		buy("A", "a1", "2025-01-05", "30", "3000"),
		sell("A", "a2", "2026-05-01", "10", "3000"), // gain 2000 @33%
		sell("A", "a3", "2026-10-06", "10", "1800"), // gain  800 @33% (last day of old rate)
		sell("A", "a4", "2026-10-07", "10", "2500"), // gain 1500 @31% (first day of new rate)
	}, true)
	ys := res.Years[0]
	eq(t, "net", ys.NetGainEUR, d("4300"))
	eq(t, "taxable", ys.TaxableEUR, d("3030"))
	if len(ys.TaxableByRate) != 2 {
		t.Fatalf("slices = %+v", ys.TaxableByRate)
	}
	// exemption 1270 eats into the first (May) gain: 2000−1270 = 730 @33%, +800 @33% = 1530 @33%; 1500 @31%
	eq(t, "33% slice", ys.TaxableByRate[0].TaxableEUR, d("1530"))
	eq(t, "31% slice", ys.TaxableByRate[1].TaxableEUR, d("1500"))
	eq(t, "tax", ys.TaxEUR, d("504.90").Add(d("465.00")))      // 1530×0.33 + 1500×0.31
	eq(t, "initial period", ys.InitialPeriodTaxEUR, ys.TaxEUR) // nothing in December
}

func TestRateBeforeTableErrors(t *testing.T) {
	_, err := Compute(Input{Trades: []model.Trade{
		buy("A", "a1", "2010-01-05", "1", "100"),
		sell("A", "a2", "2011-06-01", "1", "5000"),
	}, Rules: rules})
	if err == nil {
		t.Fatal("expected error for disposal before first rate band")
	}
}

func TestOversellFails(t *testing.T) {
	_, err := Compute(Input{Trades: []model.Trade{
		buy("A", "a1", "2024-01-05", "1", "100"),
		sell("A", "a2", "2024-06-01", "2", "300"),
	}, Rules: rules})
	if err == nil {
		t.Fatal("expected error")
	}
}
