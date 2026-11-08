package main

import (
	"context"
	"fmt"
	"io"
	"strings"

	"github.com/ichenhe/cgt-ie/internal/classify"
	"github.com/ichenhe/cgt-ie/internal/config"
	"github.com/ichenhe/cgt-ie/internal/model"
)

// resolved is the outcome of classifying every instrument in a set of trades.
type resolved struct {
	// Classifications has one entry per distinct instrument: the user's
	// override, a confident register-based result, or an unresolved guess.
	Classifications []classify.Classification
	// Instruments is the subset of Classifications safe to compute with
	// (user overrides and confident results), keyed by Instrument.Key().
	Instruments map[string]model.Instrument
	// Unresolved are the classifications that need a human decision.
	Unresolved []classify.Classification
	// Warnings about the register lookup itself (network down, ...).
	Warnings []string
}

// resolveInstruments classifies the instruments in trades. The user's
// overrides win; the rest are looked up in the regulatory registers.
func resolveInstruments(ctx context.Context, trades []model.Trade, known config.Overrides, refCacheFile string) resolved {
	out := resolved{Instruments: map[string]model.Instrument{}}

	// Only instruments without an override need the registers.
	var isins []string
	seen := map[string]struct{}{}
	for _, t := range trades {
		if _, dup := seen[t.ISIN]; dup || t.ISIN == "" {
			continue
		}
		seen[t.ISIN] = struct{}{}
		if _, ok := known.Lookup(t); !ok {
			isins = append(isins, t.ISIN)
		}
	}
	var infos map[string]classify.Info
	if len(isins) > 0 {
		registers := classify.Cached(refCacheFile, classify.Chain(classify.NewFIRDS(nil), classify.NewGLEIF(nil)))
		var err error
		infos, err = registers.Query(ctx, isins)
		if err != nil {
			out.Warnings = append(out.Warnings, fmt.Sprintf(
				"instrument register lookup incomplete (%v); classification fell back to name heuristics for some instruments", err))
		}
	}

	out.Classifications = classify.ClassifyAll(trades, known, infos)
	for _, c := range out.Classifications {
		if c.Confident {
			out.Instruments[c.Instrument.Key()] = c.Instrument
		} else {
			out.Unresolved = append(out.Unresolved, c)
		}
	}
	return out
}

// classificationNotes turns classifications into --instruments entries: the
// instrument plus a note holding the evidence, prefixed with CHECK when the
// result still needs a decision.
func classificationNotes(cls []classify.Classification) ([]model.Instrument, map[string]string) {
	list := make([]model.Instrument, 0, len(cls))
	notes := make(map[string]string, len(cls))
	for _, c := range cls {
		list = append(list, c.Instrument)
		note := c.Source + ": " + c.Reason
		if !c.Confident {
			note = "CHECK (alternatives: cgt | exit_tax) " + note
		}
		notes[c.Instrument.Key()] = note
	}
	return list, notes
}

// writeClassifications records classifications in the YAML shape
// --instruments reads.
func writeClassifications(path string, cls []classify.Classification) error {
	list, notes := classificationNotes(cls)
	return config.WriteInstruments(path, list, notes)
}

// unresolvedError explains the instruments that need a human decision and
// prints a YAML block the user can paste into their --instruments file.
func unresolvedError(unresolved []classify.Classification, instrPath string) error {
	var b strings.Builder
	fmt.Fprintf(&b, "%d instrument(s) need your classification before the report can be computed:\n\n", len(unresolved))
	for _, c := range unresolved {
		fmt.Fprintf(&b, "  %-8s %-13s %s\n           %s\n\n", c.Instrument.Ticker, c.Instrument.Key(), trunc(c.Instrument.Name, 50), c.Reason)
	}
	if instrPath == "" {
		instrPath = "instruments.yaml"
		fmt.Fprintf(&b, "Save the following as %s, set each regime, and pass --instruments %s:\n\n", instrPath, instrPath)
	} else {
		fmt.Fprintf(&b, "Add the following to %s and set each regime:\n\n", instrPath)
	}
	list, notes := classificationNotes(unresolved)
	if y, err := config.MarshalInstruments(list, notes); err == nil {
		b.Write(y)
	}
	return fmt.Errorf("%s", b.String())
}

// Unlike writeClassifications, printClassifications prints a table summary
// instead of program-readable yaml config.
func printClassifications(w io.Writer, cls []classify.Classification) {
	fmt.Fprintf(w, "%-8s %-13s %-9s %-6s %-6s %s\n", "TICKER", "KEY", "REGIME", "SURE", "SOURCE", "REASON")
	for _, c := range cls {
		sure := "yes"
		if !c.Confident {
			sure = "CHECK"
		}
		fmt.Fprintf(w, "%-8s %-13s %-9s %-6s %-6s %s\n", c.Instrument.Ticker, c.Instrument.Key(), c.Instrument.Regime, sure, c.Source, c.Reason)
	}
}
