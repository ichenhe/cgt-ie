package t212

import (
	"os"
	"strings"
	"testing"
	"time"

	"github.com/shopspring/decimal"

	"github.com/ichenhe/cgt-ie/internal/broker"
	"github.com/ichenhe/cgt-ie/internal/model"
)

func d(s string) decimal.Decimal { return decimal.RequireFromString(s) }

// wantUTC checks that got is exactly the given instant in UTC.
func wantUTC(t *testing.T, got time.Time, y int, mo time.Month, dd, h, mi, sec int) {
	t.Helper()
	want := time.Date(y, mo, dd, h, mi, sec, 0, time.UTC)
	if !got.Equal(want) || got.Location() != time.UTC {
		t.Errorf("time = %v (%v), want %v UTC", got, got.Location(), want)
	}
}

// Parse then convert, as the CLI does, so TotalEUR is populated.
func parseAndConvert(t *testing.T, in string) broker.Result {
	t.Helper()
	res, err := Parse(strings.NewReader(in), "test.csv")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := broker.ConvertToEUR(res.Trades, nil); err != nil {
		t.Fatal(err)
	}
	return res
}

const header2026 = `Action,Time (UTC),ISIN,Ticker,Name,Notes,ID,No. of shares,Price / share,Currency (Price / share),Exchange rate,Result,Currency (Result),Total,Currency (Total),Withholding tax,Currency (Withholding tax),Currency conversion from amount,Currency (Currency conversion from amount),Currency conversion to amount,Currency (Currency conversion to amount),Currency conversion fee,Currency (Currency conversion fee),Merchant name,Merchant category`

func TestParse2026Format(t *testing.T) {
	in := header2026 + "\n" + `Market sell,2026-01-02 14:30:21+00:00,US11135F1012,AVGO,"Broadcom",,EOF44308845146,0.4257406200,352.7500000000,USD,1.17263999,6.53,"EUR",127.88,"EUR",,,,,,,0.19,"EUR",,
Market buy,2026-02-26 19:30:20+00:00,US67066G1040,NVDA,"Nvidia",,EOF47211903932,0.5395780400,185.3300000000,USD,1.00000000,,,100.00,"USD",,,,,,,,,,
Dividend (Dividend),2026-02-12 15:19:54+00:00,US0378331005,AAPL,"Apple",,,5.5045445100,0.182000,USD,0.84241000,,,0.84,"EUR",0.43,"USD",,,,,,,,
Currency conversion,2026-07-29 13:56:15+00:00,,,,"2000.00 EUR -> 2278.20 USD",SVWLLW94O3O1XMR2,,,,,,,-3.42,"USD",,,2000.00,"EUR",2278.20,"USD",-3.42,"USD",,
Card debit,2026-01-03 15:27:12+00:00,,,,,8a1b1f77,,,,,,,-37.60,"EUR",,,,,,,,,"Aldi",RETAIL_STORES
`
	res, err := Parse(strings.NewReader(in), "2026.csv")
	if err != nil {
		t.Fatal(err)
	}
	if len(res.Trades) != 3 || len(res.Skipped) != 2 {
		t.Fatalf("trades=%d skipped=%d", len(res.Trades), len(res.Skipped))
	}
	sell := res.Trades[0]
	if sell.ID != "EOF44308845146" || sell.Ticker != "AVGO" || sell.Kind != model.Sell {
		t.Errorf("sell = %+v", sell)
	}
	// Total 127.88 net of 0.19 fee → gross 128.07
	if !sell.Gross.Amount.Equal(d("128.07")) || len(sell.Fees) != 1 || !sell.Fees[0].Amount.Equal(d("0.19")) {
		t.Errorf("sell gross=%+v fees=%+v", sell.Gross, sell.Fees)
	}
	if !sell.HasBrokerResult || !sell.BrokerResult.Equal(d("6.53")) {
		t.Errorf("result = %s", sell.BrokerResult)
	}
	wantUTC(t, sell.Time, 2026, time.January, 2, 14, 30, 21)

	buy := res.Trades[1]
	if buy.Gross.CCY != "USD" || !buy.Gross.Amount.Equal(d("100")) || !buy.NeedsFX() || len(buy.Fees) != 0 {
		t.Errorf("usd buy = %+v", buy)
	}
	if dv := res.Trades[2]; dv.Kind != model.Dividend || dv.ISIN != "US0378331005" {
		t.Errorf("dividend = %+v", dv)
	}
}

// Earlier exports named the column "Time" without offset (2006-01-02 15:04:05)
// rather than "Time (UTC)".
func TestLegacyTimeColumn(t *testing.T) {
	in := `Action,Time,ISIN,Ticker,Name,No. of shares,Price / share,Currency (Price / share),Exchange rate,Result,Currency (Result),Total,Currency (Total),Notes,ID,Currency conversion fee,Currency (Currency conversion fee)
buy ,2024-01-03 12:59:03,IE00B5BMR087,CSPX,"iShares Core S&P 500",10,1,EUR,1,,"EUR",10,"EUR",,T1,0,"EUR"
buy ,2024-01-03 12:59:03,IE00B5BMR087,CSPX,"iShares Core S&P 500",10,1,EUR,1,,"EUR",10,"EUR",,T1,0,"EUR"
sell,2024-01-05 00:00:00,IE00B5BMR087,CSPX,"iShares Core S&P 500",8,2,EUR,1,,"EUR",16,"EUR",,T2,0,"EUR"
`
	res, err := Parse(strings.NewReader(in), "old.csv")
	if err != nil {
		t.Fatal(err)
	}
	m := broker.Merge(res)
	// 2 trades have same ID "T1"
	if len(m.Trades) != 2 || m.Duplicates != 1 {
		t.Fatalf("trades=%d dups=%d", len(m.Trades), m.Duplicates)
	}
	if m.Trades[0].Kind != model.Buy || m.Trades[1].Kind != model.Sell {
		t.Errorf("kinds wrong: %+v", m.Trades)
	}
	wantUTC(t, m.Trades[0].Time, 2024, time.January, 3, 12, 59, 3)
	if len(m.Trades[0].Fees) != 0 {
		t.Errorf("no fees expected: %+v", m.Trades[0].Fees)
	}
}

// Older exports suffix amount columns with the account currency.
func TestParseLegacySuffixedColumns(t *testing.T) {
	in := `Action,Time,ISIN,Ticker,Name,No. of shares,Price / share,Currency (Price / share),Exchange rate,Result (EUR),Total (EUR),Withholding tax,Currency (Withholding tax),Charge amount (EUR),Stamp duty reserve tax (EUR),Notes,ID,Currency conversion fee (EUR)
Market buy,2022-03-07 14:30:01,GB0002634946,BA,"BAE Systems",2,700,GBX,0.84,,20.00,,,,0.08,,L1,0.03
Stock split open,2022-07-18 00:00:00,US02079K3059,GOOGL,"Alphabet",1,2000,USD,1.0,,0,,,,,,S1,
Transfer in,2022-08-01 00:00:00,US0378331005,AAPL,"Apple",3,150,USD,1.0,,0,,,,,,T1,
`
	res := parseAndConvert(t, in)
	if len(res.Trades) != 3 {
		t.Fatalf("trades = %d", len(res.Trades))
	}
	b := res.Trades[0]
	if b.Gross.CCY != "EUR" || !b.Gross.Amount.Equal(d("19.89")) {
		t.Errorf("gross = %+v", b.Gross)
	}
	if len(b.Fees) != 2 || !b.TotalEUR.Equal(d("20.00")) {
		t.Errorf("fees=%+v total=%s", b.Fees, b.TotalEUR)
	}
	if res.Trades[1].Kind != model.Split || res.Trades[2].Kind != model.TransferIn {
		t.Errorf("kinds = %s %s", res.Trades[1].Kind, res.Trades[2].Kind)
	}
	wantUTC(t, b.Time, 2022, time.March, 7, 14, 30, 1)
}

func TestParseWarnsOnMixedCurrencyFee(t *testing.T) {
	in := header2026 + "\n" + `Market buy,2026-02-26 19:30:20+00:00,US67066G1040,NVDA,"Nvidia",,X1,1,100,USD,1.00000000,,,100.00,"USD",,,,,,,0.15,"EUR",,
`
	res, err := Parse(strings.NewReader(in), "x.csv")
	if err != nil {
		t.Fatal(err)
	}
	if len(res.Warnings) != 1 || !strings.Contains(res.Warnings[0], "EUR fee") {
		t.Errorf("warnings = %v", res.Warnings)
	}
	// Fee not backed out of the USD total; still recorded as a fee.
	tr := res.Trades[0]
	if !tr.Gross.Amount.Equal(d("100")) || len(tr.Fees) != 1 {
		t.Errorf("trade = %+v", tr)
	}
}

func TestParseNonEURSettlement(t *testing.T) {
	in := header2026 + "\n" + `Market buy,2026-02-26 19:30:20+00:00,US67066G1040,NVDA,"Nvidia",,X1,0.53957804,185.33,USD,1.00000000,,,100.00,"USD",,,,,,,,,,
`
	res, err := Parse(strings.NewReader(in), "x.csv")
	if err != nil {
		t.Fatal(err)
	}
	tr := res.Trades[0]
	if !tr.NeedsFX() || tr.Gross.CCY != "USD" || !tr.Gross.Amount.Equal(d("100")) || !tr.TotalEUR.IsZero() {
		t.Errorf("trade = %+v", tr)
	}
	if _, err := broker.ConvertToEUR(res.Trades, nil); err == nil {
		t.Error("expected error without a converter")
	}
}

func TestParseFilesSortsAndDedupesAcrossFiles(t *testing.T) {
	dir := t.TempDir()
	a := header2026 + "\n" + `Market buy,2025-03-01 10:00:00+00:00,US0378331005,AAPL,"Apple",,A1,1,100,USD,1.1,,,91,"EUR",,,,,,,,,,
Market buy,2025-01-01 10:00:00+00:00,US0378331005,AAPL,"Apple",,A0,1,100,USD,1.1,,,90,"EUR",,,,,,,,,,
`
	b := header2026 + "\n" + `Market buy,2025-03-01 10:00:00+00:00,US0378331005,AAPL,"Apple",,A1,1,100,USD,1.1,,,91,"EUR",,,,,,,,,,
Market sell,2025-04-01 10:00:00+00:00,US0378331005,AAPL,"Apple",,A2,2,120,USD,1.1,30,"EUR",218,"EUR",,,,,,,,,,
`
	pa, pb := dir+"/a.csv", dir+"/b.csv"
	mustWrite(t, pa, a)
	mustWrite(t, pb, b)

	res, err := ParseFiles([]string{pa, pb})
	if err != nil {
		t.Fatal(err)
	}
	if res.Duplicates != 1 || len(res.Trades) != 3 {
		t.Fatalf("dups=%d trades=%d", res.Duplicates, len(res.Trades))
	}
	ids := []string{res.Trades[0].ID, res.Trades[1].ID, res.Trades[2].ID}
	if ids[0] != "A0" || ids[1] != "A1" || ids[2] != "A2" {
		t.Errorf("order = %v", ids)
	}
	if res.Trades[2].Source.File != pb {
		t.Errorf("source file = %q", res.Trades[2].Source.File)
	}
}

func mustWrite(t *testing.T, path, content string) {
	t.Helper()
	if err := writeFile(path, content); err != nil {
		t.Fatal(err)
	}
}

func writeFile(path, content string) error {
	return os.WriteFile(path, []byte(content), 0o644)
}
