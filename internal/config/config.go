package config

import (
	"fmt"
	"os"
	"sort"
	"strings"
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

// instrumentsFile is the on-disk shape of an --instruments file.
type instrumentsFile struct {
	Instruments []instrumentEntry `yaml:"instruments"`
}

type instrumentEntry struct {
	// An entry is matched to trades by isin when given; otherwise by ticker,
	// narrowed to one broker if broker is given.
	ISIN   string `yaml:"isin,omitempty"`
	Ticker string `yaml:"ticker,omitempty"`
	Broker string `yaml:"broker,omitempty"`
	Name   string `yaml:"name,omitempty"`
	Regime string `yaml:"regime"`
	Note   string `yaml:"note,omitempty"`
}

// Overrides are the user-assigned classifications.
type Overrides []model.Instrument

// Lookup returns the override matching a trade: by ISIN first, then by
// ticker (and broker, if the entry names one). The result carries the
// trade's own identifiers, and its name where the entry has none, so that
// Instrument.Key() equals Trade.Key().
func (o Overrides) Lookup(t model.Trade) (model.Instrument, bool) {
	match := func(item model.Instrument) (model.Instrument, bool) {
		item.ISIN, item.Broker, item.Ticker = t.ISIN, t.Source.Broker, t.Ticker
		if item.Name == "" {
			item.Name = t.Name
		}
		return item, true
	}
	if t.ISIN != "" {
		for _, item := range o {
			if strings.EqualFold(item.ISIN, t.ISIN) {
				return match(item)
			}
		}
	}
	for _, item := range o {
		if item.ISIN != "" || !strings.EqualFold(item.Ticker, t.Ticker) {
			continue
		}
		if item.Broker != "" && !strings.EqualFold(item.Broker, t.Source.Broker) {
			continue
		}
		return match(item)
	}
	return model.Instrument{}, false
}

// LoadInstruments reads an yaml file.
func LoadInstruments(path string) (Overrides, error) {
	b, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	var f instrumentsFile
	if err := yaml.Unmarshal(b, &f); err != nil {
		return nil, fmt.Errorf("parse %s: %w", path, err)
	}
	out := make(Overrides, 0, len(f.Instruments))
	seenISIN := map[string]struct{}{}
	for i, e := range f.Instruments {
		if e.ISIN == "" && e.Ticker == "" {
			return nil, fmt.Errorf("%s: instruments[%d] needs isin or ticker", path, i)
		}
		reg := model.Regime(e.Regime)
		if reg != model.RegimeCGT && reg != model.RegimeExitTax {
			return nil, fmt.Errorf("%s: instruments[%d] (%s%s) regime must be %q or %q, got %q",
				path, i, e.ISIN, e.Ticker, model.RegimeCGT, model.RegimeExitTax, e.Regime)
		}

		isin := strings.ToUpper(e.ISIN)
		if isin != "" {
			if _, ex := seenISIN[isin]; ex {
				return nil, fmt.Errorf("%s: duplicate isin %s", path, isin)
			}
			seenISIN[isin] = struct{}{}
		}
		out = append(out, model.Instrument{ISIN: isin, Broker: e.Broker, Ticker: e.Ticker, Name: e.Name, Regime: reg})
	}
	return out, nil
}

// MarshalInstruments renders instruments in the YAML shape LoadInstruments
// reads. notes, keyed by Instrument.Key(), become each entry's note field.
func MarshalInstruments(list []model.Instrument, notes map[string]string) ([]byte, error) {
	f := instrumentsFile{}
	for _, in := range list {
		e := instrumentEntry{
			ISIN:   in.ISIN,
			Ticker: in.Ticker,
			Name:   in.Name,
			Regime: string(in.Regime),
			Note:   notes[in.Key()],
		}
		if in.ISIN == "" {
			e.Broker = in.Broker
		}
		f.Instruments = append(f.Instruments, e)
	}
	return yaml.Marshal(&f)
}

// WriteInstruments writes MarshalInstruments output to path with a header.
func WriteInstruments(path string, list []model.Instrument, notes map[string]string) error {
	b, err := MarshalInstruments(list, notes)
	if err != nil {
		return err
	}
	header := "# Instrument classifications in the format accepted by --instruments. Entries in that file override the automatic ones.\n"
	return os.WriteFile(path, append([]byte(header), b...), 0o644)
}
