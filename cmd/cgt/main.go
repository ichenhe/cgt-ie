package main

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/alecthomas/kong"

	"github.com/ichenhe/cgt-ie/internal/broker"
	"github.com/ichenhe/cgt-ie/internal/broker/t212"
)

// cli is the top level command tree.
type cli struct {
	Report   reportCmd   `cmd:"" help:"Compute CGT and exit tax and write the reports."`
	Classify classifyCmd `cmd:"" help:"Suggest instruments.yaml entries for the instruments in the exports."`
	FXUpdate fxUpdateCmd `cmd:"" name:"fx-update" help:"Download the ECB daily EUR reference rates (needed when trades were settled in USD/GBP)."`
}

type configFlags struct {
	ConfigDir   string `help:"Directory containing rules.yaml and instruments.yaml." default:"config" type:"existingdir" placeholder:"PATH"`
	Rules       string `help:"Path to rules.yaml (overrides --config-dir)." type:"existingfile" placeholder:"PATH"`
	Instruments string `help:"Path to instruments.yaml (overrides --config-dir)." type:"existingfile" placeholder:"PATH"`
}

func (c *configFlags) rulesPath() string {
	if c.Rules != "" {
		return c.Rules
	}
	return filepath.Join(c.ConfigDir, "rules.yaml")
}

func (c *configFlags) instrumentsPath() string {
	if c.Instruments != "" {
		return c.Instruments
	}
	return filepath.Join(c.ConfigDir, "instruments.yaml")
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
