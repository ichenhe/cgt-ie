package classify

import (
	"strings"
	"testing"

	"github.com/ichenhe/cgt-ie/internal/model"
)

func TestClassifyFromCFI(t *testing.T) {
	cases := []struct {
		name, isin, cfi string
		want            model.Regime
		confident       bool
	}{
		{"Apple", "US0378331005", "ESVUFR", model.RegimeCGT, true},
		{"Leonardo ADR", "US52660W1018", "EDSXFR", model.RegimeCGT, true},
		{"Scottish Mortgage", "GB00BLDYK618", "ESVUFR", model.RegimeCGT, true},
		{"iShares Core S&P 500", "IE00B5BMR087", "CEOGES", model.RegimeExitTax, true},
		{"Xtrackers MSCI World", "LU0274208692", "CECGMS", model.RegimeExitTax, true},
		{"WisdomTree Physical Gold", "JE00B1VS3770", "DAZSPR", model.RegimeCGT, true},
		{"WisdomTree 3x Leveraged", "IE00BLRPRL42", "DAZSGR", model.RegimeCGT, true},
		// US-domiciled fund: equivalence undecided → exit tax, not confident.
		{"SPDR S&P 500 ETF Trust", "US78462F1030", "CEOGES", model.RegimeExitTax, false},
	}
	for _, c := range cases {
		got := Classify(c.isin, c.isin, "", c.name, Info{CFI: c.cfi})
		if got.Instrument.Regime != c.want || got.Confident != c.confident || got.Source != "cfi" {
			t.Errorf("%s: got %s confident=%v src=%s (%s); want %s confident=%v",
				c.name, got.Instrument.Regime, got.Confident, got.Source, got.Reason, c.want, c.confident)
		}
	}
}

func TestClassifyFromIssuerWhenNoCFI(t *testing.T) {
	got := Classify("GB00B63H8491", "GB00B63H8491", "RR", "Rolls-Royce", Info{Issuer: IssuerCompany, IssuerName: "ROLLS-ROYCE HOLDINGS PLC"})
	if got.Instrument.Regime != model.RegimeCGT || !got.Confident || got.Source != "issuer" {
		t.Errorf("RR: %+v", got)
	}
	got = Classify("IE00B5BMR087", "IE00B5BMR087", "CSPX", "iShares Core S&P 500", Info{Issuer: IssuerFund, IssuerName: "ISHARES VII PLC"})
	if got.Instrument.Regime != model.RegimeExitTax || !got.Confident || got.Source != "issuer" {
		t.Errorf("CSPX via issuer: %+v", got)
	}
}

func TestClassifyFallsBackToNameAndIsNeverConfident(t *testing.T) {
	cases := []struct {
		isin, ticker, name string
		want               model.Regime
	}{
		{"US0378331005", "AAPL", "Apple", model.RegimeCGT},
		{"IE0003Z9E2Y3", "4COP", "Global X Copper Miners (Acc)", model.RegimeExitTax},
		{"JE00B1VS3770", "PHAU", "WisdomTree Physical Gold", model.RegimeCGT},
		{"US78462F1030", "SPY", "SPDR S&P 500 ETF Trust", model.RegimeExitTax},
		{"", "VUSA", "Vanguard S&P 500 UCITS ETF", model.RegimeExitTax},
	}
	for _, c := range cases {
		got := Classify(c.isin, c.isin, c.ticker, c.name, Info{})
		if got.Instrument.Regime != c.want || got.Confident || got.Source != "name" || !strings.Contains(got.Reason, "CHECK") {
			t.Errorf("%s: %+v", c.ticker, got)
		}
	}
}

func TestClassifyAllUsesInfosAndKnown(t *testing.T) {
	trades := []model.Trade{
		{ISIN: "US0378331005", Ticker: "AAPL", Name: "Apple"},
		{ISIN: "IE00B5BMR087", Ticker: "CSPX", Name: "iShares"},
		{ISIN: "GB00B63H8491", Ticker: "RR", Name: "Rolls-Royce"},
		{ISIN: "US0378331005", Ticker: "AAPL", Name: "Apple"}, // duplicate
	}
	known := map[string]model.Instrument{"GB00B63H8491": {Key: "GB00B63H8491", ISIN: "GB00B63H8491", Regime: model.RegimeCGT}}
	infos := map[string]Info{
		"US0378331005": {CFI: "ESVUFR"},
		"IE00B5BMR087": {CFI: "CEOGES"},
	}
	out := ClassifyAll(trades, known, infos)
	if len(out) != 3 {
		t.Fatalf("got %d classifications", len(out))
	}
	bySrc := map[string]string{}
	for _, s := range out {
		bySrc[s.Instrument.Ticker] = s.Source
	}
	if bySrc["AAPL"] != "cfi" || bySrc["CSPX"] != "cfi" || bySrc["RR"] != "user" {
		t.Errorf("sources = %v", bySrc)
	}
}

func TestUnclassified(t *testing.T) {
	trades := []model.Trade{
		{ISIN: "A", Ticker: "A"}, {ISIN: "B", Ticker: "B"}, {ISIN: "A", Ticker: "A"},
	}
	known := map[string]model.Instrument{"A": {Key: "A", ISIN: "A", Regime: model.RegimeCGT}}
	u := Unclassified(trades, known)
	if len(u) != 1 || u[0].ISIN != "B" {
		t.Errorf("unclassified = %+v", u)
	}
}
