package broker

import (
	"strings"
	"testing"
	"time"

	"github.com/shopspring/decimal"

	"github.com/ichenhe/cgt-ie/internal/fx"
	"github.com/ichenhe/cgt-ie/internal/model"
)

func day(s string) time.Time {
	t, _ := time.Parse("2006-01-02", s)
	return t
}

func d(s string) decimal.Decimal { return decimal.RequireFromString(s) }

func TestMergeDedupesAndSorts(t *testing.T) {
	a := Result{Trades: []model.Trade{{ID: "2", Time: day("2025-02-01")}, {ID: "1", Time: day("2025-01-01")}}}
	b := Result{Trades: []model.Trade{{ID: "2", Time: day("2025-02-01")}, {ID: "3", Time: day("2025-03-01")}}, Duplicates: 1}
	m := Merge(a, b)
	if len(m.Trades) != 3 || m.Duplicates != 2 {
		t.Fatalf("trades=%d dups=%d", len(m.Trades), m.Duplicates)
	}
	if m.Trades[0].ID != "1" || m.Trades[1].ID != "2" || m.Trades[2].ID != "3" {
		t.Errorf("order = %v %v %v", m.Trades[0].ID, m.Trades[1].ID, m.Trades[2].ID)
	}
}

// Brokers without IDs: identical rows across two overlapping exports collapse
// to one; rows with the same timestamp keep the parser's reading order.
func TestMergeSyntheticIDsAndIndexOrder(t *testing.T) {
	mk := func(file string, idx int, kind model.Kind, qty string) model.Trade {
		return model.Trade{
			Time: day("2025-05-01"), Kind: kind, Ticker: "AAPL", Qty: d(qty),
			Gross:  model.Money{Amount: d("100"), CCY: "USD"},
			Source: model.Source{Broker: "revolut", File: file, Index: idx},
		}
	}
	a := Result{Trades: []model.Trade{mk("a.csv", 0, model.Sell, "1"), mk("a.csv", 1, model.Buy, "1")}}
	b := Result{Trades: []model.Trade{mk("b.csv", 0, model.Sell, "1")}} // same as a[0]
	m := Merge(a, b)
	if len(m.Trades) != 2 || m.Duplicates != 1 {
		t.Fatalf("trades=%d dups=%d", len(m.Trades), m.Duplicates)
	}
	if m.Trades[0].Kind != model.Sell || m.Trades[1].Kind != model.Buy {
		t.Errorf("index order not kept: %v %v", m.Trades[0].Kind, m.Trades[1].Kind)
	}
	if m.Trades[0].ID == "" || m.Trades[0].Key() != "revolut:AAPL" {
		t.Errorf("synthetic id/key: %q %q", m.Trades[0].ID, m.Trades[0].Key())
	}
	if len(m.Warnings) != 0 {
		t.Errorf("cross-file duplicate should not warn: %v", m.Warnings)
	}

	// Two identical rows in the same file cannot be an overlap: still
	// de-duplicated, but with a warning naming the row.
	same := Result{Trades: []model.Trade{mk("c.csv", 0, model.Buy, "1"), mk("c.csv", 1, model.Buy, "1")}}
	same.Trades[0].Source.Row, same.Trades[1].Source.Row = 2, 3
	m2 := Merge(same)
	if len(m2.Trades) != 1 || m2.Duplicates != 1 || len(m2.Warnings) != 1 || !strings.Contains(m2.Warnings[0], "row 2") {
		t.Errorf("trades=%d dups=%d warnings=%v", len(m2.Trades), m2.Duplicates, m2.Warnings)
	}
}

func TestConvertToEUR(t *testing.T) {
	rates, err := fx.Parse(strings.NewReader("Date,USD,GBP\n2026-02-27,1.1550,0.84\n2026-02-26,1.1500,0.80\n"))
	if err != nil {
		t.Fatal(err)
	}
	trades := []model.Trade{
		// EUR buy with EUR fee: 50 + 0.10
		{ID: "a", Time: day("2026-02-26"), Kind: model.Buy, Gross: model.Money{Amount: d("50"), CCY: "EUR"}, Fees: []model.Money{{Amount: d("0.10"), CCY: "EUR"}}},
		// USD buy, no fee: 115 / 1.15 = 100
		{ID: "b", Time: day("2026-02-26"), Kind: model.Buy, Gross: model.Money{Amount: d("115"), CCY: "USD"}},
		// Saturday → Friday's rate; sell with a fee in another currency: 231/1.155 − 0.84/0.84 = 200 − 1 = 199
		{ID: "c", Time: day("2026-02-28"), Kind: model.Sell, Gross: model.Money{Amount: d("231"), CCY: "USD"}, Fees: []model.Money{{Amount: d("0.84"), CCY: "GBP"}}},
		// Non-trade kinds are left alone.
		{ID: "d", Time: day("2026-02-26"), Kind: model.Dividend, Gross: model.Money{Amount: d("9"), CCY: "USD"}},
	}
	n, err := ConvertToEUR(trades, rates)
	if err != nil {
		t.Fatal(err)
	}
	if n != 2 {
		t.Errorf("converted %d, want 2", n)
	}
	if !trades[0].TotalEUR.Equal(d("50.10")) || trades[0].FXNote != "" {
		t.Errorf("a = %s %q", trades[0].TotalEUR, trades[0].FXNote)
	}
	if !trades[1].TotalEUR.Equal(d("100")) || !strings.Contains(trades[1].FXNote, "USD 115.00 @ ECB 1.155") && !strings.Contains(trades[1].FXNote, "1.15") {
		t.Errorf("b = %s %q", trades[1].TotalEUR, trades[1].FXNote)
	}
	if !trades[2].TotalEUR.Equal(d("199")) || !strings.Contains(trades[2].FXNote, "previous business day") || !strings.Contains(trades[2].FXNote, "GBP 0.84") {
		t.Errorf("c = %s %q", trades[2].TotalEUR, trades[2].FXNote)
	}
	if !trades[3].TotalEUR.IsZero() {
		t.Errorf("dividend should be untouched: %s", trades[3].TotalEUR)
	}

	// No converter but conversion needed → error naming the row.
	_, err = ConvertToEUR([]model.Trade{{ID: "z", Kind: model.Buy, Ticker: "NVDA", Gross: model.Money{Amount: d("1"), CCY: "USD"}, Source: model.Source{Broker: "t212", File: "x.csv", Row: 7}}}, nil)
	if err == nil || !strings.Contains(err.Error(), "NVDA") || !strings.Contains(err.Error(), "row 7") {
		t.Errorf("err = %v", err)
	}
	// All EUR → nil converter is fine.
	if _, err := ConvertToEUR([]model.Trade{{ID: "y", Kind: model.Buy, Gross: model.Money{Amount: d("1"), CCY: "EUR"}}}, nil); err != nil {
		t.Error(err)
	}
}
