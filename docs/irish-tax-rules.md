# Irish taxation of shares and ETFs for a resident individual

A summary of the rules that govern how an Irish-resident individual is taxed on buying and selling listed shares and exchange-traded funds.

This is a reference, not tax advice.

Last reviewed: 7 November 2026, including Budget 2027 (7 October 2026) and Finance (No. 2) Bill 2026 (published 19 October 2026, not yet enacted).

---

## 1. Two regimes

| Instrument | Regime | Governing law |
|---|---|---|
| Shares, ADRs, REITs; ETCs and ETNs (debt-structured commodity, leveraged or inverse products); UK investment trusts | **Capital Gains Tax** | TCA 1997 Part 19: s.28, s.546, s.552, s.580, s.581, s.601 |
| Irish-domiciled UCITS ETFs held through a recognised clearing system; "equivalent" offshore funds domiciled in another EU/EEA/OECD state; US-domiciled ETFs that meet the equivalence test | **Exit tax** on offshore funds | TCA 1997 Part 27 Chapter 4: s.747B–s.747E. From 1 January 2027, Irish ETFs move to s.739DA (§3.5) |

Examples:

- An **Irish-domiciled ETF** is an investment undertaking. Ordinarily an Irish fund deducts exit tax itself before paying the investor. An ETF cannot, because its units are held in a clearing system (Clearstream, Euroclear) and trade between investors without the fund knowing who they are, so the law exempts it from deducting. The tax is not waived: the investor must self-assess it on Form 11, and the current law borrows the rules for equivalent offshore funds (s.747B–747E) to do so. (TDM Part 27-01A-03; TDM Part 27-04-01; from 2027, s.739DA)
- A **UCITS domiciled elsewhere in the EU/EEA/OECD** is an equivalent offshore fund and is taxed the same way. (s.747B(2A))
- A **US-domiciled ETF**: the USA is an OECD state with which Ireland has a treaty, so the fund is within Chapter 4 if it is "similar in all material respects" to an Irish fund (s.747B(2A); criteria in TDM 27-04-01 §2.1.1). Until 2021 Revenue confirmed that US ETFs were taxed like shares under CGT. That confirmation was withdrawn for 2022 onwards (TDM 27-01A-03, footnote 1); Revenue now requires a fund-by-fund analysis and has not published a view either way. A typical US ETF (an open-ended investment company registered under the Investment Company Act 1940, with a depositary, prospectus and SEC oversight) is likely to meet the test, in which case exit tax applies. For units held before 2022 the 8-year clock runs from 2022, so the earliest deemed disposal is in 2030; the acquisition cost is unchanged.
- **ETCs and ETNs** are debt securities, not collective investment undertakings; CGT applies.
- A **UK investment trust** is a company whose shares trade at a discount or premium to NAV; the generally accepted view is that the holding is not a "material interest" in an offshore fund (s.743(2)) and CGT applies.
- A fund domiciled **outside the EU/EEA/OECD** (Jersey, Guernsey, Cayman, …) falls under Part 27 Chapter 2 (s.747A: 40% CGT on a distributing fund, income tax otherwise). Neither regime in this document covers it.
- Assets held in an **Individual Investment Account** (Part 27A, proposed, from 1 July 2027; §3.6) are outside both regimes.

---

## 2. Capital Gains Tax on shares

### 2.1 Chargeable gain

For each disposal:

```
chargeable gain = consideration received
                − incidental costs of disposal
                − (acquisition cost + incidental costs of acquisition)
```

Incidental costs are those wholly and exclusively incurred for the acquisition or disposal: broker commission, currency conversion charges, stamp duty and transaction taxes. (s.552)

Amounts in a foreign currency are translated to EUR at the rate on the date of the transaction. The acquisition cost is fixed in EUR when the asset is acquired; a later change in the exchange rate does not alter it. (s.532; TDM Part 19-01-14)

### 2.2 Identifying which shares were sold

Shares of the same class in the same company acquired at different times are identified with a disposal in this order:

1. **Four-week rule, LIFO.** Shares acquired within the 4 weeks preceding the disposal are deemed to be the ones sold, latest acquisition first. (s.581(1))
2. **FIFO.** Any excess over those is identified with the earliest acquisitions. (s.580; s.581(2))

"Within 4 weeks preceding" is a calendar test: an acquisition 28 days before the disposal, or on the day of the disposal, falls inside it.

Average cost is not a permitted method.

### 2.3 Loss followed by reacquisition within 4 weeks

Where shares are sold at a loss and shares of the same class are **reacquired within 4 weeks after** the disposal:

- the loss is not an allowable loss in the ordinary way; it may be deducted only from a chargeable gain arising on the eventual disposal of the reacquired shares;
- if fewer shares are reacquired than were disposed of, only the corresponding fraction of the loss is restricted: the proportion `(quantity not reacquired) / (quantity disposed of)` remains an ordinary allowable loss. (s.581(3))

The rule is triggered by the reacquisition, not by the sale alone. A loss on shares that are sold and **not** reacquired within 4 weeks is an ordinary allowable loss under s.546.

Both the identification rules and the loss restriction extend to transactions by a spouse living with the taxpayer. (s.581(4))

### 2.4 Annual computation

CGT is assessed for a tax year, which is the calendar year.

```
net chargeable gains = Σ chargeable gains
                     − Σ allowable losses of the year
                     − allowable losses carried forward from earlier years
personal exemption   = the first €1,270 of net chargeable gains (individuals only)
taxable amount       = net chargeable gains − personal exemption
CGT                  = taxable amount × rate
```

- Allowable losses are set against chargeable gains of the same year first; any excess is carried forward without time limit to the next year in which there are gains. Losses cannot be carried back, except losses in the year of death, which may be set against gains of the three preceding years. (s.546; s.573)
- The €1,270 exemption cannot be carried forward and is not available to companies or trusts. (s.601)
- Losses on development land may only be set against development-land gains. (s.653)
- Spouses jointly assessed may set one spouse's losses against the other's gains unless either elects otherwise by 1 April of the following year. (s.1028)

### 2.5 Rate

| Date of disposal | Rate |
|---|---|
| 6 December 2012 – 6 October 2026 | 33% |
| 7 October 2026 onwards | **31%** |

The reduction to 31% was announced in Budget 2027 on 7 October 2026 and applies to disposals made on or after that date (Finance (No. 2) Bill 2026 s.36, amending s.28; Revenue, *How to calculate CGT*, already shows 31%). Disposals of development land, and of unquoted shares deriving most of their value from such land, remain at 33%. (s.28)

> **Mid-year rate change.** Where a year contains gains at both rates, neither the statute nor published guidance states which gains the personal exemption and allowable losses are to be set against first. Revenue's return form for 2026 is expected to address this. Until then, any allocation is an assumption.

### 2.6 Payment and filing

| Date of disposal | Payment due |
|---|---|
| 1 January – 30 November ("initial period") | 15 December of the same year |
| 1 – 31 December ("later period") | 31 January of the following year |

The return for the year is due by **31 October of the following year**, regardless of the payment dates and even if no tax is payable because of losses or the exemption. (s.959AN; Revenue, *When and how do you pay and file CGT?*)

Return forms: Form CG1 for a person who does not otherwise file a tax return; Form 12 for a PAYE taxpayer who files one; Form 11 for a self-assessed person. Payment is made through ROS or myAccount after registering for CGT. Late payment attracts interest; a late return attracts a surcharge.

### 2.7 Related rules not summarised here

Bonus and rights issues (treated as acquired with the original holding, cost apportioned); shares of different classes (cost apportioned by market value); indexation relief for assets acquired before 2003; gifts and disposals at market value; losses on disposals to connected persons; credit for foreign CGT; shares acquired through employee share schemes (RSUs, ESPP), where income tax applies at vesting and CGT on the later disposal.

---

## 3. Exit tax on Irish, EU and US UCITS ETFs

### 3.1 Chargeable events

Each of the following is a chargeable event for the investor:

- a disposal of units, including a redemption or a transfer (s.747E(1));
- a distribution (dividend) by the fund (s.747D);
- the **8th anniversary** of the acquisition of the units, and each 8th anniversary after that, while the units are still held (the "deemed disposal", s.747E(6)).

A switch between sub-funds of the same umbrella scheme is not a disposal (s.747E(1), definition of "umbrella scheme"), but the original cost must be tracked across the switch.

### 3.2 Computation

- On a disposal, the gain is computed as it would be for CGT but without indexation relief (s.747E(1A)): `consideration − incidental costs of disposal − (cost + incidental costs of acquisition)`, all in EUR, cost fixed at acquisition.
- On a distribution, the amount of the distribution.
- On a deemed disposal, `market value on the anniversary − cost`; under s.747E(6) the units are treated as reacquired at that market value.

Distributions are charged to **income tax under Case III** and disposals under **Case IV**, both at the flat rate in §3.3. USC and PRSI do not apply (s.531AM(1)(b)(iii)(VII)). The amount is not a chargeable gain for CGT purposes, so:

- the €1,270 personal exemption does not apply;
- CGT losses cannot reduce it and it cannot be reduced by CGT losses;
- **a loss on a fund is not allowable** against anything: not against a gain on another fund, not against a later gain on the same fund, not against other income, and it cannot be carried forward. Each chargeable event is taxed on its own. (s.747E(1A), (3), (4); TDM Part 27-04-01 §4.1.6)

**Credit for deemed-disposal tax.** On the eventual actual disposal, Revenue's practice is to compute the gain from the original acquisition cost and set off the tax already paid on deemed disposals; the total tax must not exceed the tax on the actual disposal, and where the actual gain is nil the overpaid amount is refundable (s.747E(1A), (3)(b); TDM Part 27-04-01 §4.1.4). The statute's "reacquired at market value" wording and Revenue's "use the original cost" practice reach the same total tax; the s.739DA regime from 2027 (§3.5) adopts the original-cost method explicitly.

Identification of units: FIFO (TDM Part 27-04-01 §4.1.5; statutory under s.739DA(2) from 2027). s.747E(1A) imports CGT computation rules, so whether s.581 also applies is arguable; Revenue's practice is FIFO.

### 3.3 Rate

| Date of chargeable event | Rate for an individual |
|---|---|
| to 31 December 2025 | 41% |
| 1 January 2026 – 31 December 2026 | 38% |
| 1 January 2027 onwards | **35%** |

Sources: s.747D–747E as amended by Finance Act 2013 (41%); Finance Act 2025 s.37 (38%; TDM Part 27-04-01 updated January 2026); Finance (No. 2) Bill 2026 s.29 (35%, for events on or after 1 January 2027). A rate of 60% applies to a personal portfolio investment undertaking, i.e. a fund whose investments the investor can select or influence; a listed index ETF is not one. (s.739BA)

### 3.4 Payment and filing

Income under Case III/IV is returned and paid under self-assessment with the income tax return, **Form 11**, by 31 October of the year following the chargeable event (with the usual ROS extension). The CGT payment dates do not apply, but the ordinary preliminary-tax rules for Form 11 filers do: preliminary tax for the year is due by 31 October of that year. Acquiring a material interest in an offshore fund makes an individual a chargeable person for self-assessment purposes (s.747C). (TDM Part 27-04-01 §4.1.1–4.1.2)

Offshore fund income is entered in Panel E of Form 11 under "Offshore Funds (Part 27 Ch 4)"; s.747C requires the acquisition itself to be reported in the return for the year of acquisition.

### 3.5 From 1 January 2027: s.739DA for Irish ETFs (Finance (No. 2) Bill 2026, s.28)

The Bill inserts a new TCA s.739DA for units in an Irish investment undertaking that are held in a recognised clearing system ("relevant fund units"), which is how every Irish-domiciled ETF is held. Under s.79(8) of the Bill it comes into operation on 1 January 2027. Investors in such units will no longer use Chapter 4; funds domiciled in other EU/EEA/OECD states (and US funds, if equivalent) remain under s.747B–747E.

What s.739DA provides:

- Chargeable events ("relevant fund unit events"): a distribution; a redemption; a transfer of the units by sale or otherwise; the end of each 8-year period from acquisition. (s.739DA(1))
- Excluded events, with the cost carried over: an exchange of units within the same undertaking or between sub-funds of the same umbrella; transfers between spouses or civil partners and the other transfers in s.739B(1) "chargeable event" (IV)(A)–(DA). (s.739DA(1), (7))
- Identification: FIFO within a class. (s.739DA(2))
- Gain on redemption or transfer: `proceeds − incidental costs of disposal − (cost + incidental costs of acquisition)`; on a distribution, the full amount; on an 8-year event, `value − cost`. Incidental costs are fees, commission and costs of transfer. (s.739DA(3), (4))
- A negative result is treated as nil and no loss arises for any purpose. (s.739DA(5))
- All gains are Case IV income: 35% for individuals, 60% for a PPIU, 25% for companies; not reckoned in total income and no reliefs or deductions against it. (s.739DA(8))
- Deemed disposal credit: a later event is computed **as if the earlier 8-year event had not occurred** (original cost, no rebasing); tax paid on the earlier event is set off in full against a redemption, transfer or later 8-year event, and pro rata against a distribution; any excess is repaid on a claim made in the return. Tax paid under Chapter 4 before 2027 counts as creditable earlier tax. (s.739DA(9))
- The investor is a chargeable person for the year of the event and the return must state the fund's name and address, the event and the gain. (s.739DA(10))

The Bill had not been enacted at the time of writing; Finance Acts are normally signed in late December.

### 3.6 Individual Investment Account (proposed, Part 27A)

The same Bill (s.26) creates an Individual Investment Account, announced for 1 July 2027 and subject to a commencement order. One account per Irish-resident adult, contributions capped at €12,000 a year; eligible investments include listed shares, bonds and UCITS ETFs. Income and gains on assets in the account are exempt from income tax, CGT and exit tax. Instead a 1% "investment account tax" is charged each year on the average value of the account above a €50,000 threshold, calculated, withheld and reported by the account provider. Transactions inside such an account should be excluded from any CGT or exit tax computation.

---

## 4. Foreign currency held as cash

Foreign currency is a chargeable asset. Buying USD with EUR is an acquisition; spending the USD, including on shares, is a disposal of that USD, and any movement in the EUR/USD rate between the two dates is a chargeable gain or allowable loss within CGT. A foreign-currency bank or broker account is treated as a single asset, with FIFO identification of amounts paid in and out. (s.532; s.541; TDM Part 19-01-14)

Acceptable rates for translating foreign amounts: the rate actually obtained in the transaction, or the Central Bank/ECB reference rate for the date. Revenue also accepts the Central Bank's annual average rate provided the same method is applied consistently.

---

## 5. Records

The taxpayer must keep sufficient records to support the return for six years and produce the computation on request. For share disposals that means, per disposal: date, instrument, quantity, consideration in EUR, the acquisition(s) with which it was identified and their cost in EUR, the identification rule applied, and any s.581(3) restriction. (s.886)

---

## 6. Sources

Revenue's published guidance is the primary source throughout. Section numbers in the text (s.581, s.747E, …) refer to the Taxes Consolidation Act 1997 and are given so a point can be looked up in the Notes for Guidance; only s.581 is linked, because §2.3 contrasts its wording with Revenue's web page.

**Revenue — Capital Gains Tax on the disposal of an asset** (web pages)
- [How to calculate CGT](https://www.revenue.ie/en/gains-gifts-and-inheritance/transfering-an-asset/how-to-calculate-cgt.aspx) — exemption, allowable expenses, rate table including 31% from 7 October 2026
- [When and how do you pay and file CGT?](https://www.revenue.ie/en/gains-gifts-and-inheritance/transfering-an-asset/when-and-how-do-you-pay-and-file-cgt.aspx)
- [If you make a loss](https://www.revenue.ie/en/gains-gifts-and-inheritance/transfering-an-asset/if-you-make-a-loss.aspx)
- [Selling or disposing of shares](https://www.revenue.ie/en/gains-gifts-and-inheritance/transfering-an-asset/selling-or-disposing-of-shares.aspx) — FIFO, four-week rules, worked examples

**Revenue — Tax and Duty Manuals** (PDF)

- [Part 19-01-14 Foreign currency](https://www.revenue.ie/en/tax-professionals/tdm/income-tax-capital-gains-tax-corporation-tax/part-19/19-01-14.pdf)
- [Part 19-04-03 Disposals of marketable shares and securities (s.581)](https://www.revenue.ie/en/tax-professionals/tdm-wm/income-tax-capital-gains-tax-corporation-tax/part-19/19-04-03.pdf)
- [Part 27-01A-03 Exchange Traded Funds](https://www.revenue.ie/en/tax-professionals/tdm/income-tax-capital-gains-tax-corporation-tax/part-27/27-01a-03.pdf) — reviewed May 2025; footnote 1 withdraws the shares-like treatment of US ETFs from 1 January 2022
- [Part 27-04-01 Offshore Funds: EU, EEA and OECD member states](https://www.revenue.ie/en/tax-professionals/tdm/income-tax-capital-gains-tax-corporation-tax/part-27/27-04-01.pdf) — updated January 2026; equivalence criteria, exit tax rate, Case III/IV, losses, deemed disposal, filing
- [Notes for Guidance on the TCA 1997](https://www.revenue.ie/en/tax-professionals/legislation/notes-for-guidance/taxes-consolidation-act-tca/index.aspx) — current wording of any section cited above; [Part 27](https://www.revenue.ie/en/tax-professionals/documents/notes-for-guidance/tca/part27.pdf) for s.747B–747E

**Statute**
- [TCA 1997 s.581](https://www.irishstatutebook.ie/eli/1997/act/39/section/581/enacted/en/html) — four-week rules, as enacted

**Not yet enacted or in Revenue guidance**
- [Finance (No. 2) Bill 2026](https://assets.gov.ie/static/documents/89edf30b/Finance_No._2_Bill_2026.pdf) (No. 98 of 2026) and its [Explanatory Memorandum](https://data.oireachtas.ie/ie/oireachtas/bill/2026/98/eng/memo/b9826d-memo.pdf) — s.26 Individual Investment Account (Part 27A); s.28 new s.739DA for Irish ETFs; s.29 exit tax 35%; s.36 CGT 31%
- Summary: [KPMG — Investment funds and life assurance products](https://kpmg.com/ie/en/insights/tax/finance-bill-2026/investment-funds-life-assurance.html)
