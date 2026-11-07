// Package model holds the broker-agnostic domain types shared by every
// other package. Nothing in here knows about CSV files, YAML, or tax rules.
package model

import (
	"fmt"
	"strings"
	"time"

	"github.com/shopspring/decimal"
)

// Kind is what a broker row represents. Only Buy and Sell reach the tax
// engines today; the other kinds are parsed so that parsers do not have to
// change when rules for them are added.
type Kind string

const (
	Buy         Kind = "buy"
	Sell        Kind = "sell"
	TransferIn  Kind = "transfer_in"  // shares arriving from another broker; not an acquisition
	TransferOut Kind = "transfer_out" // shares leaving; not a disposal
	Split       Kind = "split"        // quantity restatement, no tax event
	Dividend    Kind = "dividend"     // cash distribution (income tax / exit tax on fund distributions)
	Fee         Kind = "fee"          // standalone fee row
	Other       Kind = "other"        // deposits, interest, card spend, ...; never a trade
)

// IsTrade reports whether the kind is a buy or sell.
func (k Kind) IsTrade() bool { return k == Buy || k == Sell }

// Regime is the Irish tax regime an instrument falls under.
type Regime string

const (
	RegimeCGT     Regime = "cgt"
	RegimeExitTax Regime = "exit_tax"
)

// Money is an amount in a named currency. Amounts are stored as exported by
// the broker; conversion to EUR happens once, in broker.ConvertToEUR.
type Money struct {
	Amount decimal.Decimal
	CCY    string
}

// IsEUR reports whether no conversion is needed.
func (m Money) IsEUR() bool { return m.CCY == "" || m.CCY == "EUR" }

func (m Money) String() string { return m.CCY + " " + m.Amount.StringFixed(2) }

// Source says where a row was read from, for error messages and for stable
// ordering of same-timestamp rows. It takes no part in de-duplication.
type Source struct {
	Broker string // parser name, e.g. "t212"
	File   string
	Row    int // 1-based line in the file
	Index  int // position among rows the parser emitted, in reading order
}

func (s Source) String() string {
	if s.File == "" {
		return s.Broker
	}
	return fmt.Sprintf("%s row %d of %s", s.Broker, s.Row, s.File)
}

// Trade is one broker row that touches a holding: a buy, a sell, or one of
// the other Kinds. Amounts are in their original currency until
// broker.ConvertToEUR fills TotalEUR.
type Trade struct {
	ID     string // broker's own ID; empty if the broker has none (Merge synthesises one)
	Time   time.Time
	Kind   Kind
	ISIN   string // may be empty for brokers that do not export it; see Key()
	Ticker string
	Name   string
	Qty    decimal.Decimal
	// Gross is the consideration excluding incidental costs: shares × price,
	// in the currency the broker settled in.
	Gross Money
	// Fees are the incidental costs of the row (commission, FX fee, stamp
	// duty, FTT, ...), each in its own currency. For a buy they add to cost;
	// for a sell they reduce proceeds.
	Fees []Money
	// TotalEUR is the all-in EUR figure used by the engines: cost including
	// fees for a buy, net proceeds for a sell. Filled by broker.ConvertToEUR.
	TotalEUR decimal.Decimal
	// FXNote records how TotalEUR was derived when any component was not
	// already in EUR, e.g. "USD 100.00 @ ECB 1.1814 (2026-02-26)".
	FXNote string

	Source Source

	// Informational, not used in the calculation.
	Price           Money
	BrokerFX        decimal.Decimal // the broker's own exchange-rate column, if any
	BrokerResult    decimal.Decimal // the broker's own P&L column, for cross-checking
	HasBrokerResult bool
}

// Key identifies the security. ISIN when the broker exports one; otherwise
// "broker:ticker", which the user maps in instruments.yaml.
func (t Trade) Key() string {
	if t.ISIN != "" {
		return t.ISIN
	}
	return t.Source.Broker + ":" + t.Ticker
}

// NeedsFX reports whether any component is in a non-EUR currency.
func (t Trade) NeedsFX() bool {
	if !t.Gross.IsEUR() {
		return true
	}
	for _, f := range t.Fees {
		if !f.IsEUR() {
			return true
		}
	}
	return false
}

// FeesTotalIn sums fees in the given currency (others are ignored).
func (t Trade) FeesTotalIn(ccy string) decimal.Decimal {
	sum := decimal.Zero
	for _, f := range t.Fees {
		if strings.EqualFold(f.CCY, ccy) || (ccy == "EUR" && f.IsEUR()) {
			sum = sum.Add(f.Amount)
		}
	}
	return sum
}

// Day truncates the trade time to a UTC calendar date. All "within 4 weeks"
// and "8 years after" comparisons work on dates, not timestamps.
func (t Trade) Day() time.Time {
	return Day(t.Time)
}

// Instrument is the user-confirmed classification of a security.
type Instrument struct {
	Key    string // ISIN or broker:ticker; what Trade.Key() returns
	ISIN   string
	Ticker string
	Name   string
	Regime Regime
}
