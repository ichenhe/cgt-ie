# cgt-ie

Irish **Capital Gains Tax** (shares) and **exit tax** (EU UCITS ETFs) calculator. Currently supports  Trading 212 CSV exports.

> [!IMPORTANT]
> Calculation aid, not tax advice. Verify the result before filing. The authors and contributors accept no liability for calculation errors.


## Usage

1. Export your **complete** history CSV from Trading 212 (History → Export). If one export does not cover everything, export several. Missing buy transactions will result in compute failure because the cost can not be determined.

2. Classify every instrument once:

   ```sh
   ./bin/cgt classify -o config/instruments.suggested.yaml exports/*.csv
   ```

   Classification uses the instrument's legal form from public regulatory registers: the ISO 10962 CFI code in ESMA FIRDS (`E` equity, `D` debt such as ETCs, `C` fund), then the issuer category in GLEIF. Results are cached locally, so only new instruments need the network. Clear cases (shares, ADRs, ETCs, EU/EEA UCITS ETFs) are adopted automatically and `report` runs without this step; it stops only on instruments that need a human decision, chiefly **US-domiciled ETFs**, where since 1 January 2022 Revenue takes no view on whether the fund is "equivalent" (exit tax) or not (CGT). Review the `CHECK` lines, then save the file as `config/instruments.yaml`; entries there always win over the automatic result.

   Instruments are keyed by ISIN. For a broker whose export has no ISIN, the key is `broker:ticker` (e.g. `key: revolut:AAPL`) and you supply the ISIN in the entry.

3. Run the report:

   ```sh
   ./bin/cgt report exports/*.csv                       # all transactions in exports
   ./bin/cgt report --year 2025 exports/*.csv           # one year
   ./bin/cgt report --no-loss-restriction exports/*.csv # disable s.581(3)
   ./bin/cgt report --losses-forward 1500 exports/*.csv # CGT losses from before your first export
   ```

## Implemented rules

See [docs/irish-tax-rules.md](docs/irish-tax-rules.md) for detailed rules this tool references.

| | Shares | Irish/EU UCITS ETFs (and US ETFs by default) |
|---|---|---|
| Regime | CGT, s.28 / s.601 | Exit tax on offshore funds, s.747E (s.739DA for Irish ETFs from 2027) |
| Identification | 4-week LIFO then FIFO (s.581(1)(2)) | FIFO |
| Rate | 33% to 6 Oct 2026, 31% from 7 Oct 2026 (by disposal date) | 41% ≤ 2025, 38% from 2026, 35% from 2027 (by disposal date) |
| Annual exemption | €1,270 | none |
| Losses | allowable; s.581(3) 4-week reacquisition restriction applied; carried forward | **not allowable** according to the law |
| 8-year deemed disposal | n/a | dates and open lots listed; tax is not calculated due to missing price on the due date |

## Limitations

The following cases are not handled:

- Stock splits, share transfers between brokers, dividends: parsed and warnings, but not processed.
- Spouse's transactions (s.581(4)).
- CGT on foreign-currency cash balances themselves (gains from exchange rate fluctuations).
- Deemed disposal identified but no gain computation (needs external prices). After a deemed disposal the lot keeps its original cost; the credit for tax paid on it is not tracked.
- US-domiciled ETFs are classified as exit tax by default (Revenue withdrew the shares-like treatment from 2022) but equivalence under s.747B must be confirmed per fund, and the 2022 restart of the 8-year clock for pre-2022 holdings is not applied.
- Transfers in/out.

## Build & Tests

```sh
go build -o bin/cgt ./cmd/cgt

go test ./...
```

Test cases include Revenue's examples for s.581(3), partial reacquisition, pro-rata release, window boundaries, carry-forward with exemption.
