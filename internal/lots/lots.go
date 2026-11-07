// Package lots implements share identification: which acquisition each
// disposal is matched against. It knows nothing about tax rates.
//
// Rules implemented (TCA 1997 s.580, s.581(1)(2)):
//   - Shares acquired within the matching window (4 weeks) before a disposal
//     are matched first, most recent acquisition first (LIFO).
//   - Anything left is matched against the oldest holdings (FIFO).
//
// The window can be disabled (set to 0) for exit-tax funds, where s.581
// does not apply and plain FIFO is used.
package lots

import (
	"fmt"
	"time"

	"github.com/shopspring/decimal"

	"github.com/ichenhe/cgt-ie/internal/model"
)

// Precision used for proportional cost splits (decimal.Div default is 16).
const divPrecision = 12

// Rule records which identification rule produced a match.
type Rule string

const (
	RuleFIFO   Rule = "FIFO"
	RuleLIFO4W Rule = "LIFO_4W"
)

// A Lot is one acquisition (a buy) and what is left of it. Qty and CostEUR
// shrink as later sells are matched against it; the Orig* fields never change.
// It carries information to determine "which acquisition(s) should a disposal come from?".
type Lot struct {
	Key         string // model.Trade.Key(): ISIN or broker:ticker
	TradeID     string
	AcquiredAt  time.Time // date (midnight UTC)
	OrigQty     decimal.Decimal
	OrigCostEUR decimal.Decimal
	Qty         decimal.Decimal // remaining
	CostEUR     decimal.Decimal // remaining cost attributable to Qty
	// FXNote is copied from the acquisition trade when its cost had to be
	// converted from a non-EUR settlement.
	FXNote string
	// RestrictedLossEUR is a s.581(3) loss carried on this lot: a loss on an
	// earlier disposal that may only be deducted from a gain on disposing
	// of these reacquired shares. Managed by the cgt engine.
	RestrictedLossEUR decimal.Decimal
}

// Closed reports whether the lot has been fully disposed of (sold).
func (l *Lot) Closed() bool { return l.Qty.Sign() <= 0 }

// take removes qty from the lot and returns the cost attributable to it.
func (l *Lot) take(qty decimal.Decimal) decimal.Decimal {
	if qty.GreaterThanOrEqual(l.Qty) {
		cost := l.CostEUR
		l.Qty = decimal.Zero
		l.CostEUR = decimal.Zero
		return cost
	}
	cost := l.CostEUR.Mul(qty).DivRound(l.Qty, divPrecision)
	l.Qty = l.Qty.Sub(qty)
	l.CostEUR = l.CostEUR.Sub(cost)
	return cost
}

// Match is one slice of a disposal answers the question "which acquisition did
// this disposal come from? And why?"
// A disposal (sell) that spans several lots produces one Match per lot.
type Match struct {
	Lot         *Lot
	Qty         decimal.Decimal
	CostEUR     decimal.Decimal // cost carried from the lot for Qty shares
	ProceedsEUR decimal.Decimal // share of net sale
	Rule        Rule            // how the match was identified
}

// GainEUR is proceeds minus cost (negative for a loss).
func (m Match) GainEUR() decimal.Decimal { return m.ProceedsEUR.Sub(m.CostEUR) }

// Book holds the open lots for one ISIN in acquisition (buy) order.
type Book struct {
	Key  string
	Lots []*Lot // acquisition order; closed lots are kept for audit
}

// NewBook creates an empty book.
func NewBook(key string) *Book { return &Book{Key: key} }

// Acquire appends a lot.
func (b *Book) Acquire(t model.Trade) *Lot {
	l := &Lot{
		Key:         t.Key(),
		TradeID:     t.ID,
		AcquiredAt:  t.Day(),
		OrigQty:     t.Qty,
		OrigCostEUR: t.TotalEUR,
		Qty:         t.Qty,
		CostEUR:     t.TotalEUR,
		FXNote:      t.FXNote,
	}
	b.Lots = append(b.Lots, l)
	return l
}

// Open returns the open (not fully sold) lots in acquisition order.
func (b *Book) Open() []*Lot {
	var out []*Lot
	for _, l := range b.Lots {
		if !l.Closed() {
			out = append(out, l)
		}
	}
	return out
}

// HeldQty is the total open quantity.
func (b *Book) HeldQty() decimal.Decimal {
	sum := decimal.Zero
	for _, l := range b.Open() {
		sum = sum.Add(l.Qty)
	}
	return sum
}

// Dispose identifies a sale against open lots and returns the matches.
// windowDays > 0 enables the s.581 LIFO window; 0 means plain FIFO.
// Proceeds are apportioned to matches pro rata by quantity.
func (b *Book) Dispose(t model.Trade, windowDays int) ([]Match, error) {
	if t.Kind != model.Sell {
		return nil, fmt.Errorf("Dispose called with a %s trade", t.Kind)
	}
	if held := b.HeldQty(); t.Qty.GreaterThan(held) {
		return nil, fmt.Errorf("%s %s: selling %s but only %s held (missing earlier buys? check your exports cover the full history)",
			t.Day().Format("2006-01-02"), t.Ticker, t.Qty, held)
	}

	remaining := t.Qty
	var matches []Match

	add := func(l *Lot, qty decimal.Decimal, rule Rule) {
		cost := l.take(qty)
		proceeds := t.TotalEUR.Mul(qty).DivRound(t.Qty, divPrecision)
		matches = append(matches, Match{Lot: l, Qty: qty, CostEUR: cost, ProceedsEUR: proceeds, Rule: rule})
		remaining = remaining.Sub(qty)
	}

	// Step 1: acquisitions within the window before the disposal, LIFO.
	if windowDays > 0 {
		day := t.Day()
		windowStart := day.AddDate(0, 0, -windowDays)
		open := b.Open()
		for i := len(open) - 1; i >= 0 && remaining.Sign() > 0; i-- {
			l := open[i]
			// "within 4 weeks preceding": acquired on or after windowStart and
			// before (or on) the disposal day. Same-day acquisitions count.
			if l.AcquiredAt.Before(windowStart) || l.AcquiredAt.After(day) {
				continue
			}
			add(l, decimal.Min(remaining, l.Qty), RuleLIFO4W)
		}
	}

	// Step 2: FIFO for the rest.
	for _, l := range b.Open() {
		if remaining.Sign() <= 0 {
			break
		}
		add(l, decimal.Min(remaining, l.Qty), RuleFIFO)
	}

	// Fix rounding drift so Σ proceeds == TotalEUR exactly.
	if len(matches) > 0 {
		sum := decimal.Zero
		for _, m := range matches {
			sum = sum.Add(m.ProceedsEUR)
		}
		if diff := t.TotalEUR.Sub(sum); !diff.IsZero() {
			matches[len(matches)-1].ProceedsEUR = matches[len(matches)-1].ProceedsEUR.Add(diff)
		}
	}
	return matches, nil
}
