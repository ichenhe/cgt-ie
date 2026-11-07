package main

import (
	"fmt"
	"os"

	"github.com/ichenhe/cgt-ie/internal/fx"
)

type fxUpdateCmd struct {
	OutputFile string `short:"o" help:"Where to store the ECB rates CSV." default:"${default_fx_path}" type:"path"`
}

func (c *fxUpdateCmd) Run() error {
	fmt.Fprintf(os.Stderr, "downloading %s ...\n", fx.ECBHistURL)
	if err := fx.Download(c.OutputFile); err != nil {
		return err
	}
	r, err := fx.Load(c.OutputFile)
	if err != nil {
		return err
	}
	fmt.Fprintf(os.Stderr, "wrote %s (latest rate date %s)\n", c.OutputFile, r.Latest().Format("2006-01-02"))
	return nil
}
