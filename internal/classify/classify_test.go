package classify

import (
	"testing"

	"github.com/ichenhe/cgt-ie/internal/model"
)

func TestGuess(t *testing.T) {
	cases := []struct {
		isin, ticker, name string
		want               model.Regime
	}{
		{"US0378331005", "AAPL", "Apple", model.RegimeCGT},
		{"GB00B63H8491", "RR", "Rolls-Royce", model.RegimeCGT},
		{"IE00B5BMR087", "CSPX", "iShares Core S&P 500 UCITS ETF USD (Acc)", model.RegimeExitTax},
		{"IE0031442068", "IUSA", "iShares S&P 500", model.RegimeExitTax},
		{"IE00B3XXRP09", "VUSA", "Vanguard S&P 500", model.RegimeExitTax},
		{"LU0274208692", "XDWD", "Xtrackers MSCI World", model.RegimeExitTax},
		{"US78462F1030", "SPY", "SPDR S&P 500 ETF Trust", model.RegimeExitTax},
		{"JE00B1VS3770", "PHAU", "WisdomTree Physical Gold", model.RegimeCGT},
		{"IE00BLRPRL42", "3USL", "WisdomTree S&P 500 3x Daily Leveraged", model.RegimeCGT},
		{"GB00BLDYK618", "SMT", "Scottish Mortgage Investment Trust", model.RegimeCGT},
		{"IE00BMTX1Y45", "I500", "iShares S&P 500 Swap (Acc)", model.RegimeExitTax},
		{"IE0003Z9E2Y3", "4COP", "Global X Copper Miners (Acc)", model.RegimeExitTax},
		{"US84615Q1031", "SPCX", "SpaceX", model.RegimeCGT},
	}
	for _, c := range cases {
		got := Guess(c.isin, c.isin, c.ticker, c.name)
		if got.Instrument.Regime != c.want {
			t.Errorf("%s %q: got %s (%s), want %s", c.ticker, c.name, got.Instrument.Regime, got.Reason, c.want)
		}
	}
}

func TestUnclassified(t *testing.T) {
	trades := []model.Trade{
		{ISIN: "A", Ticker: "A"}, {ISIN: "B", Ticker: "B"}, {ISIN: "A", Ticker: "A"},
		{Ticker: "NOISIN", Source: model.Source{Broker: "revolut"}},
	}
	known := map[string]model.Instrument{
		"A":              {Key: "A", ISIN: "A", Regime: model.RegimeCGT},
		"revolut:NOISIN": {Key: "revolut:NOISIN", Regime: model.RegimeCGT},
	}
	u := Unclassified(trades, known)
	if len(u) != 1 || u[0].ISIN != "B" {
		t.Errorf("unclassified = %+v", u)
	}
	g := Guess("revolut:VUSA", "", "VUSA", "Vanguard S&P 500 UCITS ETF")
	if g.Confident || g.Instrument.Key != "revolut:VUSA" {
		t.Errorf("no-ISIN fund should be flagged CHECK: %+v", g)
	}
	us := Guess("US78462F1030", "US78462F1030", "SPY", "SPDR S&P 500 ETF Trust")
	if us.Confident || us.Instrument.Regime != model.RegimeExitTax {
		t.Errorf("US fund should default to exit tax and be flagged CHECK: %+v", us)
	}
	je := Guess("JE00XXXXXXX1", "JE00XXXXXXX1", "JEF", "Some Jersey UCITS ETF (Acc)")
	if je.Confident {
		t.Errorf("Jersey fund should be flagged CHECK: %+v", je)
	}
}
