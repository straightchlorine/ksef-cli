---
title: "Convert an Invoify invoice to FA(3) XML for KSeF"
description: "ksef convert turns an Invoify JSON or XML export into an FA(3) invoice: VAT-exempt, or with a rate of 23, 8 or 5%. Custom fields, options and the NBP mid rate."
---

# Invoices from Invoify

## `ksef convert [OPTIONS] INVOICE [OUT.xml]`

Converts an [Invoify](https://github.com/al1abb/invoify) export (JSON or
XML) into an FA(3) invoice. The result goes to standard output or to the
given file. The invoice is VAT-exempt (art. 113) unless Invoify has a percentage
tax of 23, 8 or 5. Invoices with a discount or a shipping cost are refused.

```bash
ksef convert invoice.json invoice.xml
ksef send invoice.xml
```

Invoify has no fields for tax ids, so `convert` reads them from custom
inputs:

| Party    | Field      | Meaning                                           |
|----------|------------|---------------------------------------------------|
| sender   | `NIP`      | The seller's NIP. Required.                       |
| sender   | `REGON`    | REGON, placed in the footer.                      |
| sender   | `SWIFT`    | SWIFT code of the account (also as `BIC`).        |
| receiver | `NIP`      | The buyer's Polish NIP.                           |
| receiver | `VAT`      | EU VAT number with a country prefix, `DE123...`.  |
| receiver | `NrID`     | Another foreign identifier.                       |
| receiver | `KodKraju` | The buyer's two-letter country code.              |

The `VAT` field may also be called `NrVatUE`, `VAT UE` or `VAT ID`. With no
identifier at all, the buyer is marked as an entity without one. The
buyer's country is a two-letter code (`DE`) or a name in Polish or English
(`Niemcy`, `Germany`).

A NIP is checked: 10 digits and the check digit. A foreign number typed into
the `NIP` field is refused. A VAT number from outside the EU (e.g. `GB...`)
goes onto the invoice as another foreign identifier.

Also carried over from the Invoify invoice: item descriptions, payment
terms, the order number and every other custom field. They go onto the invoice
as additional descriptions (key and value pairs), and the order number into
the transaction terms.

### An invoice with VAT

When the Invoify invoice has a percentage tax of 23, 8 or 5, `convert` writes
an invoice with VAT. Item prices are then net prices, the tax is computed on
the net sum, and the Invoify invoice total is the gross amount.

- One rate for the whole invoice.
- PLN invoices only.
- Tax given as an amount, another rate, or the `--exemption` option end with
  an error.

Options go before the files. They cover values Invoify has no fields for:

| Option               | Meaning                                                  |
|----------------------|----------------------------------------------------------|
| `--sale-date DATE`   | Sale date, when it differs from the invoice date.        |
|                      | `prev-month-end`: the last day of the previous month.    |
| `--paid`             | Marks the invoice as paid today.                         |
| `--paid-date DATE`   | Marks the invoice as paid on the given day.              |
| `--due-date DATE`    | Payment due date, instead of the invoice's.              |
| `--place TEXT`       | Place of issue.                                          |
| `--send`             | After converting, asks and then submits to KSeF.         |
| `--payment FORM`     | Payment form: `cash`, `card`, `transfer`, `mobile`,      |
|                      | `cheque`, `credit` or `voucher`.                         |
| `--rate RATE`        | Exchange rate, instead of NBP's.                         |
| `--rate-date DATE`   | NBP's mid rate for that day, or the last one before it.  |
| `--buyer-country XX` | The buyer's country code, when the name is not known.    |
| `--buyer-id TEXT`    | The buyer's foreign identifier.                          |
| `--unit TEXT`        | Unit of measure of the lines. Defaults to `m-c` (month). |
| `--exemption TEXT`   | Legal basis of the exemption. Defaults to art. 113(1).   |

A foreign-currency invoice gets NBP's mid rate (table A, public API) from
the last working day before the invoice date. `--rate-date` picks another
day and `--rate` gives the rate directly.

An example of a euro invoice for a customer in Germany:

```bash
ksef convert --sale-date 2026-10-31 invoice.json invoice.xml
```

### Convert and submit in one command

```bash
ksef convert --send invoice.json
```

`convert --send` writes the invoice as `invoice.xml` next to the source file,
shows its summary and asks whether to submit it. It submits only on the
answer `y`. No answer, for example in a script, ends with an error and the
invoice is not submitted.

`convert` does not overwrite the file of an invoice that already has a UPO.

Options you use regularly can be written once in `ksef.conf`. Command-line
parameters take precedence. Put values with a space in quotes, e.g.
`--place "Nowy Sącz"`:

```
convert_options=--sale-date prev-month-end --paid
```

After converting, `convert` shows a summary of the invoice: number, dates,
parties, items, total with the rate, and payment. The `extra` line names the
additional descriptions, and `not set` names optional values that were left
out.

Review the resulting XML before submitting it. The issuer is responsible
for the content of the invoice.
