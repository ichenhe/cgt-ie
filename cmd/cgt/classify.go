package main

import (
	"context"
	"fmt"
	"os"
	"sort"

	"github.com/ichenhe/cgt-ie/internal/classify"
	"github.com/ichenhe/cgt-ie/internal/config"
	"github.com/ichenhe/cgt-ie/internal/model"
)

type classifyCmd struct {
	configFlags
	OutputFile string   `short:"o" help:"Write classifications to this YAML file (default: print to stdout)." type:"path" placeholder:"PATH"`
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

	res := resolveInstruments(context.Background(), parsed.Trades, known, classifyRefDataCacheFile)
	for _, w := range res.Warnings {
		fmt.Fprintln(os.Stderr, "warning:", w)
	}
	sugs := res.Classifications

	// keep known instruments
	inSugs := map[string]struct{}{}
	for _, s := range sugs {
		inSugs[s.Instrument.Key] = struct{}{}
	}
	for key, k := range known {
		if _, ex := inSugs[key]; !ex {
			sugs = append(sugs, classify.Classification{Instrument: k, Reason: "user overrides", Confident: true, Source: "user"})
		}
	}
	sort.Slice(sugs, func(i, j int) bool { return sugs[i].Instrument.Ticker < sugs[j].Instrument.Ticker })

	printClassifications(os.Stderr, sugs)

	var list []model.Instrument
	notes := map[string]string{}
	for _, s := range sugs {
		list = append(list, s.Instrument)
		note := s.Reason
		if !s.Confident {
			note = "CHECK: " + note
		}
		notes[s.Instrument.Key] = note
	}

	out := c.OutputFile
	if out == "" {
		out = "/dev/stdout"
	}
	if err := config.WriteInstruments(out, list, notes); err != nil {
		return err
	}
	if c.OutputFile != "" {
		fmt.Fprintf(os.Stderr, "\nwrote %s — review the CHECK lines, then save it as %s\n", c.OutputFile, instrPath)
	}
	if n := len(res.Unresolved); n > 0 {
		fmt.Fprintf(os.Stderr, "%d instrument(s) marked CHECK need your decision.\n", n)
	}
	return nil
}
