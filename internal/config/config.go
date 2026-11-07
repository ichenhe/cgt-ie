package config

import (
	"fmt"
	"os"
	"sort"
	"time"

	"github.com/shopspring/decimal"
	"gopkg.in/yaml.v3"

	"github.com/ichenhe/cgt-ie/internal/model"
)

// Rules is the parsed rules.yaml.
type Rules struct {
	CGT     CGTRules     `yaml:"cgt"`
	ExitTax ExitTaxRules `yaml:"exit_tax"`
}

type CGTRules struct {
	Rates           []RateBand      `yaml:"rates"`            // sorted by RateBand.From ASC on load
	AnnualExemption decimal.Decimal `yaml:"annual_exemption"` // e.g. 1270
	MatchingWindow  int             `yaml:"matching_window_days"`
	// Loss restriction window for s.581(3), normally same as matching window.
	ReacquisitionWindow int `yaml:"reacquisition_window_days"`
}

type ExitTaxRules struct {
	Rates             []RateBand `yaml:"rates"` // sorted by RateBand.From ASC on load
	DeemedDisposalYrs int        `yaml:"deemed_disposal_years"`
}

type RateBand struct {
	From time.Time       `yaml:"from"` // inclusive
	Rate decimal.Decimal `yaml:"rate"` // e.g. 0.35
}

// RateOn returns the rate whose band contains d: the entry with the latest
// From that is on or before d. Dates before the first band are an error.
func (e ExitTaxRules) RateOn(d time.Time) (decimal.Decimal, error) {
	return rateOn(e.Rates, d, "exit tax")
}

// RateOn returns the CGT rate in force on disposal date d.
func (c CGTRules) RateOn(d time.Time) (decimal.Decimal, error) {
	return rateOn(c.Rates, d, "CGT")
}

func rateOn(bands []RateBand, d time.Time, what string) (decimal.Decimal, error) {
	d = model.Day(d)
	var out decimal.Decimal
	found := false
	for _, b := range bands {
		if !model.Day(b.From).After(d) {
			out = b.Rate
			found = true
		}
	}
	if !found {
		return decimal.Zero, fmt.Errorf("no %s rate configured for %s", what, d.Format("2006-01-02"))
	}
	return out, nil
}

func sortBands(b []RateBand) {
	sort.Slice(b, func(i, j int) bool { return b[i].From.Before(b[j].From) })
}

// LoadRules reads and validates rules.yaml.
func LoadRules(path string) (Rules, error) {
	var r Rules
	b, err := os.ReadFile(path)
	if err != nil {
		return r, err
	}
	if err := yaml.Unmarshal(b, &r); err != nil {
		return r, fmt.Errorf("parse %s: %w", path, err)
	}
	if len(r.CGT.Rates) == 0 {
		return r, fmt.Errorf("%s: cgt.rates must not be empty", path)
	}
	sortBands(r.CGT.Rates)
	if r.CGT.MatchingWindow <= 0 {
		r.CGT.MatchingWindow = 28
	}
	if r.CGT.ReacquisitionWindow <= 0 {
		r.CGT.ReacquisitionWindow = r.CGT.MatchingWindow
	}
	if len(r.ExitTax.Rates) == 0 {
		return r, fmt.Errorf("%s: exit_tax.rates must not be empty", path)
	}
	sortBands(r.ExitTax.Rates)
	if r.ExitTax.DeemedDisposalYrs <= 0 {
		r.ExitTax.DeemedDisposalYrs = 8
	}
	return r, nil
}

// instrumentsFile is the on-disk shape of instruments.yaml.
type instrumentsFile struct {
	Instruments []instrumentEntry `yaml:"instruments"`
}

type instrumentEntry struct {
	// Exactly one of isin / key identifies the security. key is
	// "broker:ticker" for brokers whose exports carry no ISIN.
	ISIN   string `yaml:"isin,omitempty"`
	Key    string `yaml:"key,omitempty"`
	Ticker string `yaml:"ticker,omitempty"`
	Name   string `yaml:"name,omitempty"`
	Regime string `yaml:"regime"`
	Note   string `yaml:"note,omitempty"`
}

// Instruments maps Trade.Key() (ISIN or broker:ticker) → classification.
type Instruments map[string]model.Instrument

// LoadInstruments reads instruments.yaml. A missing file is not an error:
// it returns an empty map so the caller can report every ISIN as
// unclassified.
func LoadInstruments(path string) (Instruments, error) {
	out := Instruments{}
	b, err := os.ReadFile(path)
	if os.IsNotExist(err) {
		return out, nil
	}
	if err != nil {
		return nil, err
	}
	var f instrumentsFile
	if err := yaml.Unmarshal(b, &f); err != nil {
		return nil, fmt.Errorf("parse %s: %w", path, err)
	}
	for i, e := range f.Instruments {
		key := e.ISIN
		if key == "" {
			key = e.Key
		}
		if key == "" {
			return nil, fmt.Errorf("%s: instruments[%d] needs isin or key", path, i)
		}
		reg := model.Regime(e.Regime)
		switch reg {
		case model.RegimeCGT, model.RegimeExitTax:
		default:
			return nil, fmt.Errorf("%s: instruments[%d] (%s) regime must be %q or %q, got %q",
				path, i, key, model.RegimeCGT, model.RegimeExitTax, e.Regime)
		}
		if _, dup := out[key]; dup {
			return nil, fmt.Errorf("%s: duplicate instrument %s", path, key)
		}
		out[key] = model.Instrument{Key: key, ISIN: e.ISIN, Ticker: e.Ticker, Name: e.Name, Regime: reg}
	}
	return out, nil
}

// WriteInstruments writes a list of instruments in the same YAML shape that
// LoadInstruments reads. Used by the classifier to emit a suggestions file.
func WriteInstruments(path string, list []model.Instrument, notes map[string]string) error {
	f := instrumentsFile{}
	for _, in := range list {
		e := instrumentEntry{
			ISIN:   in.ISIN,
			Ticker: in.Ticker,
			Name:   in.Name,
			Regime: string(in.Regime),
			Note:   notes[in.Key],
		}
		if in.ISIN == "" {
			e.Key = in.Key
		}
		f.Instruments = append(f.Instruments, e)
	}
	b, err := yaml.Marshal(&f)
	if err != nil {
		return err
	}
	header := "# Suggested classification. REVIEW EVERY LINE, then save as instruments.yaml.\n"
	return os.WriteFile(path, append([]byte(header), b...), 0o644)
}
