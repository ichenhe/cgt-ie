package main

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/ichenhe/cgt-ie/internal/classify"
	"github.com/ichenhe/cgt-ie/internal/config"
	"github.com/ichenhe/cgt-ie/internal/model"
)

// resolved is the outcome of classifying every instrument in a set of trades.
type resolved struct {
	// Instruments is the user's instruments.yaml + every confident
	// automatic classification. Safe to compute with.
	Instruments     config.Instruments
	Classifications []classify.Classification
	// Unresolved are the suggestions that need a human decision.
	Unresolved []classify.Classification
	// Warnings about the lookup itself (network down, register unreachable).
	Warnings []string
}

// resolveInstruments classifies the instruments in trades. Anything in known wins.
func resolveInstruments(ctx context.Context, trades []model.Trade, known config.Instruments, refCacheFile string) resolved {
	out := resolved{Instruments: config.Instruments{}}
	for k, v := range known {
		out.Instruments[k] = v
	}

	var isins []string
	for _, t := range classify.Unclassified(trades, known) {
		if t.ISIN != "" {
			isins = append(isins, t.ISIN)
		}
	}
	var infos map[string]classify.Info
	if len(isins) > 0 {
		hc := &http.Client{Timeout: 30 * time.Second}
		registers := classify.Cached(refCacheFile, classify.Chain(classify.NewFIRDS(hc), classify.NewGLEIF(hc)))
		var err error
		infos, err = registers.Query(ctx, isins)
		if err != nil {
			out.Warnings = append(out.Warnings, fmt.Sprintf(
				"instrument register lookup incomplete (%v); classification fell back to name heuristics for some instruments", err))
		}
	}

	out.Classifications = classify.ClassifyAll(trades, known, infos)
	for _, s := range out.Classifications {
		if s.Source == "user" {
			continue
		}
		if s.Confident {
			out.Instruments[s.Instrument.Key] = s.Instrument
		} else {
			out.Unresolved = append(out.Unresolved, s)
		}
	}
	return out
}

// unresolvedError formats the instruments that need a human decision.
func unresolvedError(unresolved []classify.Classification, instrPath string, exports []string) error {
	var b strings.Builder
	fmt.Fprintf(&b, "%d instrument(s) need a solid classification before the report can be computed. Add them to %s:\n\n", len(unresolved), instrPath)
	for _, s := range unresolved {
		fmt.Fprintf(&b, "  %-8s %-13s %s\n           suggested: %s\n           %s\n\n",
			s.Instrument.Ticker, s.Instrument.Key, trunc(s.Instrument.Name, 50), s.Instrument.Regime, s.Reason)
	}
	fmt.Fprintf(&b, "Run 'cgt classify -o %s.suggested.yaml %s' to get a file with every instrument filled in, review the CHECK lines, and save it as %s.",
		strings.TrimSuffix(instrPath, ".yaml"), strings.Join(quoteAll(exports), " "), instrPath)
	return fmt.Errorf("%s", b.String())
}

func quoteAll(ss []string) []string {
	out := make([]string, len(ss))
	for i, s := range ss {
		if strings.ContainsAny(s, " '\"") {
			s = "'" + s + "'"
		}
		out[i] = s
	}
	return out
}

func printClassifications(w io.Writer, sugs []classify.Classification) {
	fmt.Fprintf(w, "%-8s %-13s %-9s %-6s %-6s %s\n", "TICKER", "ISIN", "REGIME", "SURE", "SOURCE", "REASON")
	for _, s := range sugs {
		sure := "yes"
		if !s.Confident {
			sure = "CHECK"
		}
		fmt.Fprintf(w, "%-8s %-13s %-9s %-6s %-6s %s\n", s.Instrument.Ticker, s.Instrument.Key, s.Instrument.Regime, sure, s.Source, s.Reason)
	}
}
