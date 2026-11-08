package main

import (
	"context"
	"fmt"
	"os"
)

type classifyCmd struct {
	configFlags
	OutputFile string   `short:"o" help:"Also write the classifications as a YAML file in --instruments format." type:"path" placeholder:"PATH"`
	TxFiles    []string `arg:"" name:"export.csv" help:"Trading 212 transaction export(s), CSV." type:"existingfile"`
}

func (c *classifyCmd) Run() error {
	known, err := c.loadOverrides()
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
	printClassifications(os.Stdout, res.Classifications)
	if n := len(res.Unresolved); n > 0 {
		fmt.Fprintf(os.Stderr, "\n%d instrument(s) marked CHECK need your decision.\n", n)
	}

	if c.OutputFile != "" {
		if err := writeClassifications(c.OutputFile, res.Classifications); err != nil {
			return err
		}
		fmt.Fprintf(os.Stderr, "wrote %s — review the CHECK lines, then pass it to 'cgt report --instruments %s'\n", c.OutputFile, c.OutputFile)
	}
	return nil
}
