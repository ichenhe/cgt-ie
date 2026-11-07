// Package classify guesses the tax regime of an instrument from its ISIN and
// name.
package classify

import (
	"sort"
	"strings"

	"github.com/ichenhe/cgt-ie/internal/model"
)

// Suggestion is a guessed classification with the reason.
type Suggestion struct {
	Instrument model.Instrument
	Reason     string
	Confident  bool
}

// Fund-issuer / fund-type words that strongly indicate a collective
// investment vehicle.
var fundWords = []string{
	"ucits", "etf", "ishares", "vanguard", "xtrackers", "x-trackers", "spdr",
	"invesco", "amundi", "lyxor", "hsbc msci", "hsbc ftse", "hsbc s&p",
	"ubs etf", "wisdomtree", "vaneck", "global x", "l&g", "legal & general", "jpm ",
	"jpmorgan etf", "fidelity ucits", "franklin ftse", "franklin libertyshares",
	// Share-class markers used by Trading 212 for funds, e.g. "iShares S&P 500 Swap (Acc)"
	"(acc)", "(dist)", "(acc.)", "(dis)", " acc ", " dist ",
}

// Words that indicate a debt-structured product (ETC/ETN) or a leveraged /
// inverse product. These are not funds for s.747 purposes and fall under CGT.
var etcWords = []string{
	"etc", "etn", "leveraged", "3x", "2x", "-3x", "-2x", "short ", "daily short",
	"physical gold", "physical silver", "physical platinum", "physical palladium",
	"brent", "wti", "crude oil", "natural gas", "bitcoin", "ethereum", "crypto",
}

// EU/EEA/OECD ISIN prefixes commonly used for UCITS funds. Jersey (JE) and
// Guernsey (GG) are deliberately absent: they are outside the EU/EEA/OECD, so
// a fund domiciled there falls under Part 27 Chapter 2 (s.747A), which this
// tool does not model.
var fundDomiciles = map[string]bool{
	"IE": true, "LU": true, "DE": true, "FR": true, "NL": true, "CH": true,
	"GB": true, "LI": true, "AT": true, "BE": true,
	"DK": true, "FI": true, "SE": true, "NO": true, "IT": true, "ES": true,
}

// Guess returns a suggestion for one instrument. key is Trade.Key(); isin
// may be empty for brokers that do not export it.
func Guess(key, isin, ticker, name string) Suggestion {
	prefix := strings.ToUpper(isin)
	if len(prefix) >= 2 {
		prefix = prefix[:2]
	} else {
		prefix = "??"
	}
	n := " " + strings.ToLower(name) + " "

	ins := model.Instrument{Key: key, ISIN: isin, Ticker: ticker, Name: name}

	hasFund := containsAny(n, fundWords)
	hasETC := containsAny(n, etcWords)

	switch {
	case hasETC:
		ins.Regime = model.RegimeCGT
		return Suggestion{ins, "name suggests ETC/ETN or leveraged product (debt security, not a fund) → CGT", hasFund}
	case hasFund && prefix == "US":
		// Since 1 January 2022 Revenue no longer confirms that US ETFs are
		// taxed like shares (TDM Part 27-01A-03). The USA is an OECD state,
		// so a US fund that is "similar in all material respects" to an
		// Irish fund is an equivalent offshore fund under s.747B(2A) and
		// exit tax applies. Default to the more conservative regime.
		ins.Regime = model.RegimeExitTax
		return Suggestion{ins, "US-domiciled fund → exit tax by default; since 1 Jan 2022 Revenue no longer treats US ETFs as shares (TDM 27-01A-03). CHECK s.747B equivalence; 8-year clock for pre-2022 holdings starts in 2022", false}
	case hasFund && fundDomiciles[prefix]:
		ins.Regime = model.RegimeExitTax
		return Suggestion{ins, "name indicates UCITS fund/ETF, domiciled " + prefix + " → exit tax (s.747E)", true}
	case hasFund && isin == "":
		ins.Regime = model.RegimeExitTax
		return Suggestion{ins, "name indicates a fund but the export has no ISIN; CHECK domicile (EU/EEA/OECD → exit tax; elsewhere → s.747A, not modelled)", false}
	case hasFund:
		ins.Regime = model.RegimeExitTax
		return Suggestion{ins, "name indicates a fund but domicile " + prefix + " is outside the EU/EEA/OECD list; CHECK whether s.747E or s.747A applies", false}
	default:
		ins.Regime = model.RegimeCGT
		return Suggestion{ins, "no fund keywords in name → assumed ordinary share → CGT", true}
	}
}

// SuggestAll produces one suggestion per distinct ISIN in the trades, sorted
// by ticker. Known instruments are passed through unchanged.
func SuggestAll(trades []model.Trade, known map[string]model.Instrument) []Suggestion {
	seen := map[string]bool{}
	var out []Suggestion
	for _, t := range trades {
		if seen[t.Key()] {
			continue
		}
		seen[t.Key()] = true
		if k, ok := known[t.Key()]; ok {
			if k.Ticker == "" {
				k.Ticker = t.Ticker
			}
			if k.Name == "" {
				k.Name = t.Name
			}
			out = append(out, Suggestion{Instrument: k, Reason: "already in instruments.yaml", Confident: true})
			continue
		}
		out = append(out, Guess(t.Key(), t.ISIN, t.Ticker, t.Name))
	}
	sort.Slice(out, func(i, j int) bool {
		return out[i].Instrument.Ticker < out[j].Instrument.Ticker
	})
	return out
}

// Unclassified returns trades who's instruments are not in known map.
func Unclassified(trades []model.Trade, known map[string]model.Instrument) []model.Trade {
	seen := map[string]bool{}
	var out []model.Trade
	for _, t := range trades {
		if seen[t.Key()] {
			continue
		}
		seen[t.Key()] = true
		if _, ok := known[t.Key()]; !ok {
			out = append(out, t)
		}
	}
	return out
}

func containsAny(s string, words []string) bool {
	for _, w := range words {
		if strings.Contains(s, w) {
			return true
		}
	}
	return false
}
