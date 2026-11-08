# cgt-ie

Irish **Capital Gains Tax** (shares) and **exit tax** (EU UCITS ETFs) calculator. Currently supports  Trading 212 CSV exports.

> [!IMPORTANT]
> Calculation aid, not tax advice. Verify the result before filing. The authors and contributors accept no liability for calculation errors.

## Example

[`examples/`](examples/README.md) has a small synthetic data set covering every rule (4-week LIFO, s.581(3) full and partial restriction, December payment split, the 2026 mid-year rate change, exit tax at two rates, a disallowed fund loss, an overdue deemed disposal). Also check [report it produces](examples/report/report-2024-2026.md) to see what this tool can do for you.

## Usage

1. Export your **complete** history CSV from Trading 212 (History → Export). If one export does not cover everything, export several. Missing buy transactions will result in compute failure because the cost can not be determined.

2. Run the report:

   ```sh
   ./bin/cgt report exports/*.csv                       # all transactions in exports
   ./bin/cgt report --year 2025 exports/*.csv           # one year
   ./bin/cgt report --no-loss-restriction exports/*.csv # disable s.581(3)
   ./bin/cgt report --losses-forward 1500 exports/*.csv # CGT losses from before your first export
   ```

### Classification

   Most instruments are classified automatically from public regulatory registers. However, in some cases your confirmation is required:

   - **Non-EU-domiciled ETFs**: Exit tax applies only if the fund is 'similar in all material respects' to an Irish fund (s.747B(2A)); otherwise CGT. This tool suggests exit-tax by default.
   - **Unknown instruments**: ISIN is not available in your export transactions or can not find it in the supported registers

`cgt report` stops on instruments that need a human decision and prints a ready-to-paste YAML block. Simply save it, review it, set the regime and pass it with `--instruments`: 

```sh
cgt report --instruments instruments.yaml exports/*.csv
```

Entries in that file always win over the automatic result. Every run also writes `instruments-used-<years>.yaml` to the output directory, listing how each instrument was classified and why. It is in the same format so you can edit/use it as `--instruments`.

Besides, `cgt classify exports/*.csv -o instruments.yaml` prints a classification table and writes same yaml file without computing the tax.

### Configuration

This tool has embedded default configs and simply works.

In case you want to customize tax rules, take [`internal/config/rules.default.yaml`](./internal/config/rules.default.yaml) as reference, make changes and pass your config file via `--rules`.

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
