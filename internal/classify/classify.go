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
// domiciled elsewhere (e.g. the US) is in that regime only if it is "similar
// in all material respects" to an Irish regulated fund, which the investor
// must judge; such funds are suggested as exit tax but never confident.
package classify

import (
	"fmt"
	"sort"
	"strings"

	"github.com/ichenhe/cgt-ie/internal/model"
)

// Classification is the regime decided for one instrument, with the evidence
// behind it. Only Confident results may be used without review.
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

// eea lists ISIN prefixes of EU/EEA states.
var eea = map[string]struct{}{
	"AT": {}, "BE": {}, "BG": {}, "HR": {}, "CY": {}, "CZ": {}, "DK": {},
	"EE": {}, "FI": {}, "FR": {}, "DE": {}, "GR": {}, "HU": {}, "IE": {},
	"IT": {}, "LV": {}, "LT": {}, "LU": {}, "MT": {}, "NL": {}, "PL": {},
	"PT": {}, "RO": {}, "SK": {}, "SI": {}, "ES": {}, "SE": {}, "IS": {},
	"LI": {}, "NO": {},
}

// EEADomiciled reports whether an ISIN's country prefix is an EU/EEA state.
// A fund domiciled there is presumed to be a UCITS and therefore an
// "equivalent" offshore fund.
func EEADomiciled(isin string) bool {
	_, ok := eea[prefixOf(isin)]
	return ok
}

func prefixOf(isin string) string {
	if len(isin) >= 2 {
		return strings.ToUpper(isin[:2])
	}
	return ""
}

// Classify decides the regime for one instrument from whatever is known
// about it (info may be the zero value).
func Classify(isin, broker, ticker, name string, info Info) Classification {
	ins := model.Instrument{ISIN: isin, Broker: broker, Ticker: ticker, Name: name}
	prefix := prefixOf(isin)
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
	switch {
	case EEADomiciled(ins.ISIN):
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

// Overrides supplies user-made classifications.
type Overrides interface {
	Lookup(t model.Trade) (model.Instrument, bool)
}

// ClassifyAll produces one Classification per distinct instrument in trades.
// known and infos may be nil.
func ClassifyAll(trades []model.Trade, known Overrides, infos map[string]Info) []Classification {
	seen := map[string]struct{}{}
	var out []Classification
	for _, t := range trades {
		if _, dup := seen[t.Key()]; dup {
			continue
		}
		seen[t.Key()] = struct{}{}
		if k, ok := lookup(known, t); ok {
			out = append(out, Classification{Instrument: k, Reason: "set in --instruments", Confident: true, Source: "user"})
			continue
		}
		out = append(out, Classify(t.ISIN, t.Source.Broker, t.Ticker, t.Name, infos[strings.ToUpper(t.ISIN)]))
	}
	sort.Slice(out, func(i, j int) bool {
		return out[i].Instrument.Ticker < out[j].Instrument.Ticker
	})
	return out
}

func lookup(known Overrides, t model.Trade) (model.Instrument, bool) {
	if known == nil {
		return model.Instrument{}, false
	}
	return known.Lookup(t)
}

func containsAny(s string, words []string) bool {
	for _, w := range words {
		if strings.Contains(s, w) {
			return true
		}
	}
	return false
}
