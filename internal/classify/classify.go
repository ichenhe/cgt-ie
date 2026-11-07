// Package classify decides which Irish tax regime an instrument falls under.
//
// Decision chain:
//
//  1. CFI code (ISO 10962), from a register such as ESMA FIRDS.
//     E… equity (shares, ADRs, investment trusts)         → CGT
//     D… debt (ETCs, ETNs, leveraged/inverse "ETPs")      → CGT
//     C… collective investment vehicle                    → see fundRegime
//  2. Issuer kind, from a register such as GLEIF, when no CFI is known.
//     company → CGT; fund → see fundRegime.
//  3. Words in the instrument name, not confident.
//
// fundRegime: a fund domiciled in the EU/EEA is almost always a UCITS and
// therefore an "equivalent" offshore fund taxed under exit tax. A fund
// domiciled in another OECD treaty state (e.g. US), is exit-tax-eligible only
// if it is "similar in all material respects" to an Irish regulated fund.
// This package considers all founs as exit tax by default.
package classify

import (
	"fmt"
	"sort"
	"strings"

	"github.com/ichenhe/cgt-ie/internal/model"
)

// Classification is a guessed classification with the reason.
type Classification struct {
	Instrument model.Instrument
	Reason     string
	Confident  bool
	// Source names the evidence used: "cfi", "issuer", "name" or "user".
	Source string
}

// Fund-issuer / fund-type words that strongly indicate a collective
// investment vehicle. Used only when no register has the instrument.
var fundWords = []string{
	"ucits", "etf", "ishares", "vanguard", "xtrackers", "x-trackers", "spdr",
	"invesco", "amundi", "lyxor", "hsbc msci", "hsbc ftse", "hsbc s&p",
	"ubs etf", "wisdomtree", "vaneck", "global x", "l&g", "legal & general", "jpm ",
	"jpmorgan etf", "fidelity ucits", "franklin ftse", "franklin libertyshares",
	"(acc)", "(dist)", "(acc.)", "(dis)", " acc ", " dist ",
}

// Words that indicate a debt-structured product (ETC/ETN) or a leveraged /
// inverse product. Debt securities, not funds: CGT.
var etcWords = []string{
	"etc", "etn", "leveraged", "3x", "2x", "-3x", "-2x", "short ", "daily short",
	"physical gold", "physical silver", "physical platinum", "physical palladium",
	"brent", "wti", "crude oil", "natural gas", "bitcoin", "ethereum", "crypto",
}

// eea lists ISIN prefixes of EU/EEA states. A fund domiciled here is
// presumed UCITS-equivalent.
var eea = map[string]struct{}{
	"AT": {}, "BE": {}, "BG": {}, "HR": {}, "CY": {}, "CZ": {}, "DK": {},
	"EE": {}, "FI": {}, "FR": {}, "DE": {}, "GR": {}, "HU": {}, "IE": {},
	"IT": {}, "LV": {}, "LT": {}, "LU": {}, "MT": {}, "NL": {}, "PL": {},
	"PT": {}, "RO": {}, "SK": {}, "SI": {}, "ES": {}, "SE": {}, "IS": {},
	"LI": {}, "NO": {},
}

// Classify decides the regime for one instrument from whatever is known
// about it (info may be the zero value).
func Classify(key, isin, ticker, name string, info Info) Classification {
	ins := model.Instrument{Key: key, ISIN: isin, Ticker: ticker, Name: name}
	prefix := ""
	if len(isin) >= 2 {
		prefix = strings.ToUpper(isin[:2])
	}
	// 1. CFI code.
	if info.CFI != "" {
		switch info.CFI[0] {
		case 'E':
			return Classification{withRegime(ins, model.RegimeCGT), "CFI " + info.CFI + ": equity → CGT", true, "cfi"}
		case 'D':
			return Classification{withRegime(ins, model.RegimeCGT), "CFI " + info.CFI + ": debt instrument, not a fund → CGT", true, "cfi"}
		case 'C':
			return fundRegime(ins, prefix, "CFI "+info.CFI+": collective investment vehicle", "cfi")
		default:
			return Classification{withRegime(ins, model.RegimeCGT), "CFI " + info.CFI + ": unexpected class; CHECK", false, "cfi"}
		}
	}

	// 2. Issuer kind.
	switch info.Issuer {
	case IssuerFund:
		return fundRegime(ins, prefix, "issuer "+info.IssuerName+" is a fund", "issuer")
	case IssuerCompany:
		return Classification{withRegime(ins, model.RegimeCGT), "issuer " + info.IssuerName + " is a company, not a fund → CGT", true, "issuer"}
	}

	// 3. Infer from the name, not confident.
	n := " " + strings.ToLower(name) + " "
	hasFund := containsAny(n, fundWords)
	hasETC := containsAny(n, etcWords)
	switch {
	case hasETC:
		return Classification{withRegime(ins, model.RegimeCGT), "name suggests ETC/ETN or leveraged product → CGT; CHECK", false, "name"}
	case hasFund:
		s := fundRegime(ins, prefix, "name suggests a fund", "name")
		s.Confident = false
		s.Reason += "; CHECK"
		return s
	default:
		return Classification{withRegime(ins, model.RegimeCGT), "no register data; no fund keywords in name → CGT by default; CHECK", false, "name"}
	}
}

func fundRegime(ins model.Instrument, prefix, evidence, source string) Classification {
	_, isEEA := eea[prefix]
	switch {
	case isEEA:
		return Classification{withRegime(ins, model.RegimeExitTax),
			evidence + ", domiciled " + prefix + " (EU/EEA, presumed UCITS) → exit tax", true, source}
	case prefix == "":
		return Classification{withRegime(ins, model.RegimeExitTax),
			evidence + ", domicile unknown (no ISIN); CHECK", false, source}
	default:
		return Classification{withRegime(ins, model.RegimeExitTax),
			fmt.Sprintf("%s, domiciled %s (outside EU/EEA). Consider as exit tax by default; CHECK",
				evidence, prefix), false, source}
	}
}

func withRegime(ins model.Instrument, r model.Regime) model.Instrument {
	ins.Regime = r
	return ins
}

// ClassifyAll produces one classification per distinct instrument key in trades,
// sorted by ticker. Known instruments are passed through unchanged. infos may
// be nil.
func ClassifyAll(trades []model.Trade, known map[string]model.Instrument, infos map[string]Info) []Classification {
	seen := map[string]struct{}{}
	var out []Classification
	for _, t := range trades {
		if _, se := seen[t.Key()]; se {
			continue
		}
		seen[t.Key()] = struct{}{}
		if k, ok := known[t.Key()]; ok {
			if k.Ticker == "" {
				k.Ticker = t.Ticker
			}
			if k.Name == "" {
				k.Name = t.Name
			}
			out = append(out, Classification{Instrument: k, Reason: "already in instruments.yaml", Confident: true, Source: "user"})
			continue
		}
		out = append(out, Classify(t.Key(), t.ISIN, t.Ticker, t.Name, infos[strings.ToUpper(t.ISIN)]))
	}
	sort.Slice(out, func(i, j int) bool {
		return out[i].Instrument.Ticker < out[j].Instrument.Ticker
	})
	return out
}

// Unclassified returns one trade per instrument key not present in known.
func Unclassified(trades []model.Trade, known map[string]model.Instrument) []model.Trade {
	seen := map[string]struct{}{}
	var out []model.Trade
	for _, t := range trades {
		if _, se := seen[t.Key()]; se {
			continue
		}
		seen[t.Key()] = struct{}{}
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
