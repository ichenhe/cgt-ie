# Example

Synthetic Trading 212 exports (`trading212-*.csv`, one per year, in the 2026 export format) and the report they produce (`report/`). Tickers and ISINs are real so the register-based classification works; quantities, prices and IDs are invented.

Regenerate the report with:

```sh
cgt report --today 2026-11-08 --fx examples/ecb-rates-sample.csv -o examples/report examples/trading212-*.csv
```

`--today` is fixed so the deemed-disposal status in the committed report is stable. `ecb-rates-sample.csv` is a trimmed copy of the ECB reference-rate file (USD, Mar–Jun 2026) so the run needs no download; without `--fx` the tool would fetch the full file itself.

## What the data exercises

| Rows | Rule shown |
|---|---|
| 2018 CSPX buy, partly still held | 8-year **deemed disposal** fell due on 2026-03-15; listed as overdue |
| 2024-12-10 NVDA sell | December disposal → tax due **31 Jan**, separate from the 15 Dec payment |
| 2025-03-20 NVDA sell at a loss, 2025-04-02 NVDA buy | Repurchase within 4 weeks of **all** shares sold → whole loss **restricted** (s.581(3)); released against the gain when those shares are sold in 2026 |
| 2025-06-10 PHAU sell at a loss | ETC is a debt security → **CGT**, loss **allowable** (no repurchase) |
| 2025-10-07 AAPL buy, 2025-10-20 AAPL sell | Sale within 4 weeks of a purchase → matched **LIFO** to that purchase, not FIFO |
| 2025-08-20 / 2026-02-10 CSPX sells | **Exit tax** at 41% (2025) and 38% (2026), FIFO from the 2018 lot |
| 2026-05-12 I500 sell at a loss | Loss on a fund is **disallowed**: not netted, not carried |
| 2026-09-22 and 2026-10-15 RHM sells | One year, two CGT rates: **33%** to 6 Oct 2026, **31%** from 7 Oct; exemption applied to the earlier gain first |
| 2026-10-20 NVDA sell at a loss, 2026-11-02 NVDA buy of half the quantity | **Partial** repurchase → half the loss restricted, half allowable |
| 2026-03-10 EUR→USD conversion, 2026-03-12 NVDA buy and 2026-06-18 NVDA sell settled in **USD** | No EUR amount in the export → converted at the **ECB rate** on the trade date; the rate used is recorded in the sales breakdown and the rate file is copied next to the report |
| Deposit, interest, card, dividend rows | Skipped and counted in the warnings |
