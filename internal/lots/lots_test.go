package lots

import (
	"testing"
	"time"

	"github.com/shopspring/decimal"

	"github.com/ichenhe/cgt-ie/internal/model"
)

func d(s string) decimal.Decimal { return decimal.RequireFromString(s) }

func day(s string) time.Time {
	t, err := time.Parse("2006-01-02", s)
	if err != nil {
		panic(err)
	}
	return t
}

func buy(id, date, qty, total string) model.Trade {
	return model.Trade{ID: id, ISIN: "X", Time: day(date), Kind: model.Buy, Qty: d(qty), TotalEUR: d(total)}
}

func sell(id, date, qty, total string) model.Trade {
	return model.Trade{ID: id, ISIN: "X", Time: day(date), Kind: model.Sell, Qty: d(qty), TotalEUR: d(total)}
}

func eq(t *testing.T, name string, got, want decimal.Decimal) {
	t.Helper()
	if !got.Equal(want) {
		t.Errorf("%s = %s, want %s", name, got, want)
	}
}

func TestPlainFIFO(t *testing.T) {
	b := NewBook("X")
	b.Acquire(buy("b1", "2024-01-01", "10", "10"))
	b.Acquire(buy("b2", "2024-03-01", "5", "10"))

	m1, err := b.Dispose(sell("s1", "2024-05-01", "8", "16"), 28)
	if err != nil {
		t.Fatal(err)
	}
	if len(m1) != 1 || m1[0].Rule != RuleFIFO || m1[0].Lot.TradeID != "b1" {
		t.Fatalf("m1 = %+v", m1)
	}
	eq(t, "m1 cost", m1[0].CostEUR, d("8"))   // 10 / 10 * 8
	eq(t, "m1 gain", m1[0].GainEUR(), d("8")) // 16 - 8

	m2, err := b.Dispose(sell("s2", "2024-07-01", "4", "40"), 28)
	if err != nil {
		t.Fatal(err)
	}
	if len(m2) != 2 {
		t.Fatalf("m2 = %+v", m2)
	}
	// 2 from b1 (cost 2, proceeds 20), 2 from b2 (cost 4, proceeds 20)
	eq(t, "m2[0] qty", m2[0].Qty, d("2"))
	eq(t, "m2[0] cost", m2[0].CostEUR, d("2")) // 10 / 10 * 2
	eq(t, "m2[1] qty", m2[1].Qty, d("2"))
	eq(t, "m2[1] cost", m2[1].CostEUR, d("4")) // 10 / 5 * 2
	total := m1[0].GainEUR().Add(m2[0].GainEUR()).Add(m2[1].GainEUR())
	eq(t, "total gain", total, d("42")) // (16 - 8) + (20 - 2) + (20 - 4)
	eq(t, "held", b.HeldQty(), d("3"))  // 10 + 5 - 8 -4
}

func TestFourWeekLIFOThenFIFO(t *testing.T) {
	b := NewBook("X")
	b.Acquire(buy("b1", "2024-01-01", "10", "10"))
	b.Acquire(buy("b2", "2024-01-02", "5", "10"))

	m1, err := b.Dispose(sell("s1", "2024-01-05", "8", "16"), 28)
	if err != nil {
		t.Fatal(err)
	}
	if len(m1) != 2 {
		t.Fatalf("m1 = %+v", m1)
	}
	if m1[0].Rule != RuleLIFO4W || m1[0].Lot.TradeID != "b2" || !m1[0].Qty.Equal(d("5")) {
		t.Errorf("m1[0] = %+v", m1[0])
	}
	if m1[1].Rule != RuleLIFO4W || m1[1].Lot.TradeID != "b1" || !m1[1].Qty.Equal(d("3")) {
		t.Errorf("m1[1] = %+v", m1[1])
	}
	eq(t, "m1[0] cost", m1[0].CostEUR, d("10")) // all of b2
	eq(t, "m1[1] cost", m1[1].CostEUR, d("3"))  // 10 / 10 * (8 - 5)

	m2, err := b.Dispose(sell("s2", "2024-05-01", "4", "40"), 28)
	if err != nil {
		t.Fatal(err)
	}
	if len(m2) != 1 || m2[0].Rule != RuleFIFO {
		t.Fatalf("m2 = %+v", m2)
	}
	eq(t, "m2 cost", m2[0].CostEUR, d("4"))

	total := decimal.Zero
	for _, m := range append(m1, m2...) {
		total = total.Add(m.GainEUR())
	}
	eq(t, "total gain", total, d("39")) // (10 - 10) + (6 - 3) + (40 - 4)
}

// One disposal consumes lot inside the window (LIFO) first and then the
// remainder from the old lot (FIFO).
func TestMixedLIFOThenFIFO(t *testing.T) {
	b := NewBook("X")
	b.Acquire(buy("old", "2023-06-01", "10", "10"))
	b.Acquire(buy("recent", "2024-02-20", "2", "6"))
	m, err := b.Dispose(sell("s", "2024-03-01", "5", "25"), 28)
	if err != nil {
		t.Fatal(err)
	}
	if len(m) != 2 {
		t.Fatalf("m = %+v", m)
	}
	if m[0].Rule != RuleLIFO4W || m[0].Lot.TradeID != "recent" || !m[0].Qty.Equal(d("2")) {
		t.Errorf("m[0] = %+v", m[0])
	}
	if m[1].Rule != RuleFIFO || m[1].Lot.TradeID != "old" || !m[1].Qty.Equal(d("3")) {
		t.Errorf("m[1] = %+v", m[1])
	}
	eq(t, "m[0] cost", m[0].CostEUR, d("6"))          // all of recent
	eq(t, "m[0] proceeds", m[0].ProceedsEUR, d("10")) // 25 / 5 * 2
	eq(t, "m[1] cost", m[1].CostEUR, d("3"))          // 10 / 10 * 3
	eq(t, "m[1] proceeds", m[1].ProceedsEUR, d("15")) // 25 / 5 * 3
}

// With the window disabled, the same trades are plain FIFO.
func TestWindowDisabledIsFIFO(t *testing.T) {
	b := NewBook("X")
	b.Acquire(buy("b1", "2024-01-01", "10", "10"))
	b.Acquire(buy("b2", "2024-01-02", "5", "10"))
	m, err := b.Dispose(sell("s1", "2024-01-05", "8", "16"), 0)
	if err != nil {
		t.Fatal(err)
	}
	if len(m) != 1 || m[0].Lot.TradeID != "b1" || m[0].Rule != RuleFIFO {
		t.Fatalf("m = %+v", m)
	}
}

// Multiple lots inside the window: most recent first.
func TestLIFOOrderInsideWindow(t *testing.T) {
	b := NewBook("X")
	b.Acquire(buy("old", "2024-01-01", "10", "10"))
	b.Acquire(buy("w1", "2024-02-10", "2", "20"))
	b.Acquire(buy("w2", "2024-02-20", "2", "40"))
	m, err := b.Dispose(sell("s", "2024-03-01", "3", "90"), 28)
	if err != nil {
		t.Fatal(err)
	}
	if len(m) != 2 || m[0].Lot.TradeID != "w2" || m[1].Lot.TradeID != "w1" {
		t.Fatalf("m = %+v", m)
	}
	eq(t, "m[1] qty", m[1].Qty, d("1"))
	eq(t, "m[1] cost", m[1].CostEUR, d("10"))
}

// Window boundary: exactly 28 days before is inside; 29 days is outside.
func TestWindowBoundary(t *testing.T) {
	disposeDay := "2024-03-01"
	for _, c := range []struct {
		buyDate    string
		expectRule Rule
	}{
		{"2024-02-02", RuleLIFO4W}, // 28 days before Mar 1
		{"2024-02-01", RuleFIFO},   // 29 days before
		{"2024-03-01", RuleLIFO4W}, // same day
	} {
		b := NewBook("X")
		b.Acquire(buy("old", "2023-01-01", "10", "10"))
		b.Acquire(buy("w", c.buyDate, "1", "5"))
		m, err := b.Dispose(sell("s", disposeDay, "1", "9"), 28)
		if err != nil {
			t.Fatal(err)
		}
		if m[0].Rule != c.expectRule {
			t.Errorf("buy %s: rule = %s, want %s", c.buyDate, m[0].Rule, c.expectRule)
		}
	}
}

func TestProceedsSumExactly(t *testing.T) {
	b := NewBook("X")
	b.Acquire(buy("b1", "2024-01-01", "1", "3"))
	b.Acquire(buy("b2", "2024-01-02", "1", "3"))
	b.Acquire(buy("b3", "2024-01-03", "1", "3"))
	m, err := b.Dispose(sell("s", "2024-06-01", "3", "10"), 28)
	if err != nil {
		t.Fatal(err)
	}
	sum := decimal.Zero
	for _, x := range m {
		sum = sum.Add(x.ProceedsEUR)
	}
	eq(t, "Σ proceeds", sum, d("10"))
}

func TestOversellErrors(t *testing.T) {
	b := NewBook("X")
	b.Acquire(buy("b1", "2024-01-01", "1", "3"))
	if _, err := b.Dispose(sell("s", "2024-06-01", "2", "10"), 28); err == nil {
		t.Fatal("expected error")
	}
}

// Fractional shares as in real Trading 212 pie data.
func TestFractional(t *testing.T) {
	b := NewBook("X")
	b.Acquire(buy("b1", "2025-02-03", "0.0366562", "6.00"))
	m, err := b.Dispose(sell("s", "2025-05-12", "0.0366562", "5.82"), 28)
	if err != nil {
		t.Fatal(err)
	}
	eq(t, "gain", m[0].GainEUR(), d("-0.18"))
	if !b.Lots[0].Closed() {
		t.Error("lot should be closed")
	}
}
