package main

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/alecthomas/kong"

	"github.com/ichenhe/cgt-ie/internal/broker"
	"github.com/ichenhe/cgt-ie/internal/broker/t212"
	"github.com/ichenhe/cgt-ie/internal/config"
)

// cli is the top level command tree.
type cli struct {
	Report   reportCmd   `cmd:"" help:"Compute CGT and exit tax and write the reports."`
	Classify classifyCmd `cmd:"" help:"Show how each instrument would be classified."`
	FXUpdate fxUpdateCmd `cmd:"" name:"fx-update" help:"Download the ECB daily EUR reference rates (needed when trades were settled in USD/GBP)."`
}

type configFlags struct {
	Rules       string `help:"Optional YAML overriding the built-in tax rules; omitted keys retain default value." type:"existingfile" placeholder:"PATH"`
	Instruments string `help:"Optional YAML of instrument classifications that override the automatic ones." type:"existingfile" placeholder:"PATH"`
}

// loadOverrides reads the user's optional instrument overrides.
func (c *configFlags) loadOverrides() (config.Overrides, error) {
	if c.Instruments == "" {
		return nil, nil
	}
	return config.LoadInstruments(c.Instruments)
}

func loadTrades(files []string) (broker.Result, error) {
	if len(files) == 0 {
		return broker.Result{}, fmt.Errorf("no export files given")
	}
	return t212.ParseFiles(files)
}

func main() {
	var c cli
	ctx := kong.Parse(&c,
		kong.Name("cgt"),
		kong.Description("Irish CGT (shares) and exit tax (EU ETFs) calculator."),
		kong.UsageOnError(),
		kong.Vars{"default_fx_path": filepath.Join(tmpDir, "eurofxref-hist.csv")},
	)
	ctx.FatalIfErrorf(ctx.Run())
}

var tmpDir = filepath.Join(os.TempDir(), "cgt-ie")
var classifyRefDataCacheFile = filepath.Join(tmpDir, "refdata.json")
