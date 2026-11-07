// Package cgt computes Irish Capital Gains Tax on share disposals.
//
// Rules implemented:
//   - Identification: s.581(1)(2) 4-week LIFO then FIFO.
//   - s.581(3): a loss on a disposal followed by a reacquisition of the same
//     shares within 4 weeks is restricted. The restricted part can only be
//     deducted from a gain on disposing of the reacquired shares; if the
//     reacquired quantity is smaller than the disposed quantity the loss is
//     split pro rata.
//   - Yearly: allowable losses and losses forward are set against gains,
//     then the annual exemption (s.601), then the rate.
//   - Payment periods: disposals 1 Jan–30 Nov are due 15 Dec; 1–31 Dec are
//     due 31 Jan of the following year.
package cgt

import (
	"fmt"
	"sort"
	"time"

	"github.com/shopspring/decimal"

	"github.com/ichenhe/cgt-ie/internal/config"
	"github.com/ichenhe/cgt-ie/internal/lots"
	"github.com/ichenhe/cgt-ie/internal/model"
)

// Input to Compute.
type Input struct {
	// Trades under the CGT regime, sorted by time.
	Trades []model.Trade
	Rules  config.CGTRules
	// ApplyLossRestriction toggles s.581(3). Off = plain losses, useful to
	// see how much the rule costs.
	ApplyLossRestriction bool
	// LossesForwardIn: allowable losses carried in from years before the
	// first trade in Trades (>= 0).
	LossesForwardIn decimal.Decimal
}

// Disposal is one sell trade with all related information and tax due.
type Disposal struct {
	Trade   model.Trade  // raw sell trade
	Matches []lots.Match // matched acquisitions (lots)

	ProceedsEUR decimal.Decimal // Σ Matches.ProceedsEUR
	CostEUR     decimal.Decimal // Σ Matches.CostEUR
	RawGainEUR  decimal.Decimal // ProceedsEUR - CostEUR, before any s.581(3) adjustment

	// Restricted losses from earlier disposals can only be used to deduct gains on reacquired
	// shares (when they are eventually sold, i.e. this disposal). ReleasedLossUsedEUR is the value
	// used to reduce taxable gain on this disposal.
	ReleasedLossUsedEUR decimal.Decimal
	// ReleasedLossForfeitedEUR indicates restricted losses carried on matched lots that could not
	// be used because those shares were not sold at a gain. Such losses are lost forever.
	// For audit only. Invariant: Used + Forfeited = total released from matched lots.
	ReleasedLossForfeitedEUR decimal.Decimal

	// Tax outcome, net = RawGainEUR − ReleasedLossUsedEUR:
	//   net > 0 → ChargeableGainEUR = net; the loss fields are 0.
	//   net < 0 → AllowableLossEUR + RestrictedLossEUR = −net.
	//   net = 0 → all three fields are 0.
	ChargeableGainEUR decimal.Decimal // >= 0
	AllowableLossEUR  decimal.Decimal // >= 0
	RestrictedLossEUR decimal.Decimal // >= 0

	// ReacquiredQty is the same-ISIN quantity bought within the window after
	// this disposal (only populated when the disposal is a loss).
	ReacquiredQty decimal.Decimal

	Notes []string
}

// Year returns the tax year of the disposal.
func (d *Disposal) Year() int { return d.Trade.Day().Year() }

// IsDecember reports whether the disposal falls into the later payment period.
func (d *Disposal) IsDecember() bool { return d.Trade.Day().Month() == time.December }

// YearSummary is the CG1-style computation for one tax year.
type YearSummary struct {
	Year      int
	Disposals []*Disposal

	ProceedsEUR         decimal.Decimal
	CostEUR             decimal.Decimal
	ChargeableGainsEUR  decimal.Decimal // Σ gains
	AllowableLossesEUR  decimal.Decimal // Σ in-year allowable losses
	RestrictedLossesEUR decimal.Decimal // Σ losses deferred under s.581(3)
	LossesForwardInEUR  decimal.Decimal
	NetGainEUR          decimal.Decimal // max(gains − losses − forward, 0)
	LossesForwardOutEUR decimal.Decimal
	ExemptionUsedEUR    decimal.Decimal
	TaxableEUR          decimal.Decimal // max(net − exemption, 0)
	TaxEUR              decimal.Decimal // taxable * rate

	// TaxableByRate splits TaxableEUR by the rate on the disposal date.
	// Normally one entry; two when the rate changed in the year.
	TaxableByRate []RateSlice

	// Payment split.
	InitialPeriodTaxEUR decimal.Decimal // disposals Jan–Nov, due 15 Dec
	LaterPeriodTaxEUR   decimal.Decimal // disposals Dec, due 31 Jan next year
}

// RateSlice is the part of a year's taxable gain charged at one rate.
type RateSlice struct {
	Rate       decimal.Decimal
	TaxableEUR decimal.Decimal
	TaxEUR     decimal.Decimal
}

// Result of Compute.
type Result struct {
	Disposals []*Disposal
	Years     []*YearSummary
	Books     map[string]*lots.Book
	Warnings  []string
}

// pendingRestriction is a s.581(3) loss waiting to be attached to the
// reacquired lots as they are bought.
type pendingRestriction struct {
	fromDisposalID string
	lossRemaining  decimal.Decimal
	qtyRemaining   decimal.Decimal
	afterTime      time.Time
	untilDay       time.Time
}

// Compute runs the CGT calculation.
func Compute(in Input) (*Result, error) {
	res := &Result{Books: map[string]*lots.Book{}}
	window := in.Rules.MatchingWindow
	reacqWindow := in.Rules.ReacquisitionWindow
	pending := map[string][]*pendingRestriction{}

	for _, t := range in.Trades {
		book, ok := res.Books[t.Key()]
		if !ok {
			book = lots.NewBook(t.Key())
			res.Books[t.Key()] = book
		}

		switch t.Kind {
		case model.Buy:
			lot := book.Acquire(t)
			if in.ApplyLossRestriction {
				attachPending(pending, t, lot)
			}

		case model.Sell:
			matches, err := book.Dispose(t, window)
			if err != nil {
				return nil, err
			}
			d := &Disposal{Trade: t, Matches: matches}
			d.Notes = append(d.Notes, fxNotes(t, matches)...)
			d.settle(in.ApplyLossRestriction)

			// If the disposal is a loss, check if there are reacquisitions
			// within the window and apply s.581(3).
			if loss := d.RawGainEUR.Sub(d.ReleasedLossUsedEUR).Neg(); in.ApplyLossRestriction && loss.Sign() > 0 {
				d.applyReacquisitionRule(loss, in.Trades, reacqWindow)

				// add restricted loss to pending list for future reacquisitions
				if d.RestrictedLossEUR.Sign() > 0 {
					pending[t.Key()] = append(pending[t.Key()], &pendingRestriction{
						fromDisposalID: t.ID,
						lossRemaining:  d.RestrictedLossEUR,
						qtyRemaining:   d.ReacquiredQty,
						afterTime:      t.Time,
						untilDay:       t.Day().AddDate(0, 0, reacqWindow),
					})
				}
			}
			res.Disposals = append(res.Disposals, d)

		default:
			return nil, fmt.Errorf("trade %s: unexpected kind %q", t.ID, t.Kind)
		}
	}

	years, err := summarise(res.Disposals, in)
	if err != nil {
		return nil, err
	}
	res.Years = years
	return res, nil
}

// settle computes proceeds/cost/raw gain and applies restricted losses
// carried on the matched lots. It sets ChargeableGain / AllowableLoss as if
// no reacquisition happens; applyReacquisitionRule may move part of the
// loss into RestrictedLoss later.
func (d *Disposal) settle(applyRestriction bool) {
	for i := range d.Matches {
		m := &d.Matches[i]
		d.ProceedsEUR = d.ProceedsEUR.Add(m.ProceedsEUR)
		d.CostEUR = d.CostEUR.Add(m.CostEUR)

		if !applyRestriction || m.Lot.RestrictedLossEUR.Sign() <= 0 {
			continue
		}
		// Release the share of the restricted loss attributable to the
		// quantity sold. Lot.Qty has already been reduced by take(), so the
		// pre-sale quantity is Qty + m.Qty.
		preQty := m.Lot.Qty.Add(m.Qty)
		released := m.Lot.RestrictedLossEUR
		if m.Lot.Qty.Sign() > 0 {
			released = m.Lot.RestrictedLossEUR.Mul(m.Qty).DivRound(preQty, 12)
		}
		m.Lot.RestrictedLossEUR = m.Lot.RestrictedLossEUR.Sub(released)

		// calculate how much of the released loss can be used to reduce the gain on this disposal
		gain := m.GainEUR()
		used := decimal.Zero
		if gain.Sign() > 0 {
			used = decimal.Min(gain, released)
		}
		d.ReleasedLossUsedEUR = d.ReleasedLossUsedEUR.Add(used)
		d.ReleasedLossForfeitedEUR = d.ReleasedLossForfeitedEUR.Add(released.Sub(used))
	}
	d.RawGainEUR = d.ProceedsEUR.Sub(d.CostEUR)

	if d.ReleasedLossUsedEUR.Sign() > 0 {
		d.Notes = append(d.Notes, fmt.Sprintf("s.581(3): deducted %s of restricted loss carried on reacquired shares", money(d.ReleasedLossUsedEUR)))
	}
	if d.ReleasedLossForfeitedEUR.Sign() > 0 {
		d.Notes = append(d.Notes, fmt.Sprintf("s.581(3): %s of restricted loss forfeited (reacquired shares not sold at a gain)", money(d.ReleasedLossForfeitedEUR)))
	}

	net := d.RawGainEUR.Sub(d.ReleasedLossUsedEUR)
	switch {
	case net.Sign() > 0:
		d.ChargeableGainEUR = net
	case net.Sign() < 0:
		d.AllowableLossEUR = net.Neg()
	}
}

// applyReacquisitionRule implements s.581(3) for a disposal that produced a
// loss of `loss`. It moves allowable loss into restricted loss if the same
// instrument is reacquired within the window.
func (d *Disposal) applyReacquisitionRule(loss decimal.Decimal, all []model.Trade, windowDays int) {
	if loss.Sign() <= 0 {
		panic("applyReacquisitionRule called on a non-loss disposal")
	}

	until := d.Trade.Day().AddDate(0, 0, windowDays)
	reacq := decimal.Zero
	for _, t := range all {
		if t.Key() != d.Trade.Key() || t.Kind != model.Buy {
			continue
		}
		if !t.Time.After(d.Trade.Time) || t.Day().After(until) {
			continue
		}
		reacq = reacq.Add(t.Qty)
	}
	if reacq.Sign() <= 0 {
		return
	}
	d.ReacquiredQty = decimal.Min(reacq, d.Trade.Qty)
	if reacq.GreaterThanOrEqual(d.Trade.Qty) {
		d.RestrictedLossEUR = loss
		d.AllowableLossEUR = decimal.Zero
		d.Notes = append(d.Notes, fmt.Sprintf("s.581(3): reacquired %s within %d days; whole loss %s restricted to gains on those shares",
			reacq, windowDays, money(loss)))
		return
	}
	restricted := loss.Mul(reacq).DivRound(d.Trade.Qty, 12)
	d.RestrictedLossEUR = restricted
	d.AllowableLossEUR = loss.Sub(restricted)
	d.Notes = append(d.Notes, fmt.Sprintf("s.581(3): reacquired %s of %s within %d days; %s restricted, %s allowable",
		reacq, d.Trade.Qty, windowDays, money(restricted), money(d.AllowableLossEUR)))
}

// attachPending attaches outstanding restricted losses to a newly bought lot. The potential gain
// from future disposal can consume them to reduce chargeable gain.
func attachPending(pending map[string][]*pendingRestriction, t model.Trade, lot *lots.Lot) {
	list := pending[t.Key()]
	if len(list) == 0 {
		return
	}
	var keep []*pendingRestriction
	qtyLeft := t.Qty
	for _, p := range list {
		if p.qtyRemaining.Sign() <= 0 || t.Day().After(p.untilDay) {
			continue // consumed or expired
		}
		if !t.Time.After(p.afterTime) || qtyLeft.Sign() <= 0 {
			// This acquisition can not claim this restricted loss:
			// - It is not after the disposal that created the loss. Since trades are processed in
			//   time order, it's basically an edge case where the acquisition happens at the same
			//   time as the disposal.
			// - The acquisition has no quantity left to claim the loss. Let's say previous 2
			//   disposals created 2 huge restricted loss, and this acquisition is even smaller than
			//   the first one, so the first one will be partially claimed, and the second one will
			//   be fully left.
			//
			// For both cases, keep it for later acquisitions.
			keep = append(keep, p)
			continue
		}
		q := decimal.Min(qtyLeft, p.qtyRemaining)
		var share decimal.Decimal
		if q.Equal(p.qtyRemaining) {
			share = p.lossRemaining
		} else {
			share = p.lossRemaining.Mul(q).DivRound(p.qtyRemaining, 12)
		}
		lot.RestrictedLossEUR = lot.RestrictedLossEUR.Add(share)
		p.lossRemaining = p.lossRemaining.Sub(share)
		p.qtyRemaining = p.qtyRemaining.Sub(q)
		qtyLeft = qtyLeft.Sub(q)
		if p.qtyRemaining.Sign() > 0 {
			keep = append(keep, p)
		}
	}
	pending[t.Key()] = keep
}

func summarise(disposals []*Disposal, in Input) ([]*YearSummary, error) {
	if len(disposals) == 0 {
		return nil, nil
	}
	byYear := map[int]*YearSummary{}
	minY, maxY := disposals[0].Year(), disposals[0].Year()
	for _, d := range disposals {
		y := d.Year()
		if y < minY {
			minY = y
		}
		if y > maxY {
			maxY = y
		}
		ys, ok := byYear[y]
		if !ok {
			ys = &YearSummary{Year: y}
			byYear[y] = ys
		}
		ys.Disposals = append(ys.Disposals, d)
	}

	var out []*YearSummary
	forward := in.LossesForwardIn
	for y := minY; y <= maxY; y++ {
		ys, ok := byYear[y]
		if !ok {
			ys = &YearSummary{Year: y}
		}
		ys.LossesForwardInEUR = forward

		for _, d := range ys.Disposals {
			ys.ProceedsEUR = ys.ProceedsEUR.Add(d.ProceedsEUR)
			ys.CostEUR = ys.CostEUR.Add(d.CostEUR)
			ys.ChargeableGainsEUR = ys.ChargeableGainsEUR.Add(d.ChargeableGainEUR)
			ys.AllowableLossesEUR = ys.AllowableLossesEUR.Add(d.AllowableLossEUR)
			ys.RestrictedLossesEUR = ys.RestrictedLossesEUR.Add(d.RestrictedLossEUR)
		}

		net := ys.ChargeableGainsEUR.Sub(ys.AllowableLossesEUR).Sub(forward)
		if net.Sign() < 0 {
			ys.LossesForwardOutEUR = net.Neg()
			net = decimal.Zero
		}
		ys.NetGainEUR = net
		ys.ExemptionUsedEUR = decimal.Min(net, in.Rules.AnnualExemption)
		ys.TaxableEUR = net.Sub(ys.ExemptionUsedEUR)

		// Full-year tax, gains taxed at the rate on their disposal date.
		full, err := taxGains(ys.Disposals, func(*Disposal) bool { return true }, forward, in.Rules)
		if err != nil {
			return nil, err
		}
		ys.TaxEUR = full.tax
		ys.TaxableByRate = full.slices

		// Initial period: computed as if the year ended 30 Nov.
		initial, err := taxGains(ys.Disposals, func(d *Disposal) bool { return !d.IsDecember() }, forward, in.Rules)
		if err != nil {
			return nil, err
		}
		ys.InitialPeriodTaxEUR = initial.tax
		ys.LaterPeriodTaxEUR = ys.TaxEUR.Sub(ys.InitialPeriodTaxEUR)
		if ys.LaterPeriodTaxEUR.Sign() < 0 {
			ys.LaterPeriodTaxEUR = decimal.Zero
		}

		forward = ys.LossesForwardOutEUR
		out = append(out, ys)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Year < out[j].Year })
	return out, nil
}

type taxed struct {
	tax    decimal.Decimal
	slices []RateSlice
}

// taxGains calculates the tax on the disposals selected by include. Each gain
// is taxed at the rate on its disposal date.
//
// Annual exemption and forward losses are applied so the provided disposals
// must be in the same tax year.
func taxGains(disposals []*Disposal, include func(*Disposal) bool, forward decimal.Decimal, rules config.CGTRules) (taxed, error) {
	var sel []*Disposal
	deductions := forward
	for _, d := range disposals {
		if !include(d) {
			continue
		}
		if len(sel) > 0 && d.Year() != sel[0].Year() {
			return taxed{}, fmt.Errorf("disposals from different years: %d and %d", sel[0].Year(), d.Year())
		}

		sel = append(sel, d)
		deductions = deductions.Add(d.AllowableLossEUR)
	}
	sort.SliceStable(sel, func(i, j int) bool { return sel[i].Trade.Time.Before(sel[j].Trade.Time) })

	deductions = deductions.Add(rules.AnnualExemption)
	byRate := map[string]*RateSlice{}
	var order []string
	for _, d := range sel {
		g := d.ChargeableGainEUR
		if g.Sign() <= 0 {
			continue
		}
		use := decimal.Min(g, deductions)
		deductions = deductions.Sub(use)
		taxable := g.Sub(use)
		if taxable.Sign() <= 0 {
			continue
		}
		rate, err := rules.RateOn(d.Trade.Day())
		if err != nil {
			return taxed{}, fmt.Errorf("disposal %s: %w", d.Trade.ID, err)
		}
		key := rate.String()
		rs, ok := byRate[key]
		if !ok {
			rs = &RateSlice{Rate: rate}
			byRate[key] = rs
			order = append(order, key)
		}
		rs.TaxableEUR = rs.TaxableEUR.Add(taxable)
	}
	var out taxed
	for _, k := range order {
		rs := byRate[k]
		rs.TaxEUR = rs.TaxableEUR.Mul(rs.Rate).Round(2)
		out.tax = out.tax.Add(rs.TaxEUR)
		out.slices = append(out.slices, *rs)
	}
	return out, nil
}

func money(d decimal.Decimal) string { return "€" + d.StringFixed(2) }

// fxNotes describes any ECB conversions behind a disposal's proceeds or costs.
func fxNotes(t model.Trade, matches []lots.Match) []string {
	var out []string
	if t.FXNote != "" {
		out = append(out, "proceeds: "+t.FXNote)
	}
	n := 0
	for _, m := range matches {
		if m.Lot.FXNote != "" {
			n++
		}
	}
	if n > 0 {
		out = append(out, fmt.Sprintf("cost of %d lot(s) converted at ECB rate", n))
	}
	return out
}
