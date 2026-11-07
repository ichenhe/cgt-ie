package main

import (
	"fmt"
	"os"
	"sort"

	"github.com/ichenhe/cgt-ie/internal/classify"
	"github.com/ichenhe/cgt-ie/internal/config"
	"github.com/ichenhe/cgt-ie/internal/model"
)

type classifyCmd struct {
	configFlags
	OutputFile string   `short:"o" help:"Write suggestions to this YAML file (default: print to stdout)." type:"path" placeholder:"PATH"`
	TxFiles    []string `arg:"" name:"export.csv" help:"Trading 212 transaction export(s), CSV." type:"existingfile"`
}

func (c *classifyCmd) Run() error {
	instrPath := c.instrumentsPath()
	known, err := config.LoadInstruments(instrPath)
	if err != nil {
		return err
	}
	parsed, err := loadTrades(c.TxFiles)
	if err != nil {
		return err
	}

	sugs := classify.SuggestAll(parsed.Trades, known)
	// Keep known instruments that do not appear in these exports, so the
	// output file is a complete replacement for instruments.yaml.
	inSugs := map[string]bool{}
	for _, s := range sugs {
		inSugs[s.Instrument.Key] = true
	}
	for key, k := range known {
		if !inSugs[key] {
			sugs = append(sugs, classify.Suggestion{Instrument: k, Reason: "already in instruments.yaml (not in these exports)", Confident: true})
		}
	}
	sort.Slice(sugs, func(i, j int) bool { return sugs[i].Instrument.Ticker < sugs[j].Instrument.Ticker })

	var list []model.Instrument
	notes := map[string]string{}
	fmt.Fprintf(os.Stderr, "%-8s %-13s %-9s %-5s %s\n", "TICKER", "ISIN", "REGIME", "SURE", "REASON")
	for _, s := range sugs {
		sure := "yes"
		if !s.Confident {
			sure = "CHECK"
		}
		fmt.Fprintf(os.Stderr, "%-8s %-13s %-9s %-5s %s\n", s.Instrument.Ticker, s.Instrument.Key, s.Instrument.Regime, sure, s.Reason)
		list = append(list, s.Instrument)
		notes[s.Instrument.Key] = s.Reason
	}

	out := c.OutputFile
	if out == "" {
		out = "/dev/stdout"
	}
	if err := config.WriteInstruments(out, list, notes); err != nil {
		return err
	}
	if c.OutputFile != "" {
		fmt.Fprintf(os.Stderr, "\nwrote %s — review every line, then save it as %s\n", c.OutputFile, instrPath)
	}
	return nil
}
