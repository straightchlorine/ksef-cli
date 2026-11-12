# ksef-cli

A command-line tool for submitting invoices to KSeF (Krajowy System
e-Faktur, Poland's national e-invoicing system), API 2.0.

## Installation

You need Go 1.26 or newer.

```bash
go install codeberg.org/piotrkrzysztof/ksef-cli/cmd/ksef@latest
```

The command installs the `ksef` program in `~/go/bin`.

## KSeF token

The token is generated once, by hand, in the KSeF taxpayer app (Aplikacja
Podatnika KSeF). Each environment has its own instance and its own tokens.

1. Log in to the taxpayer app of the chosen environment (addresses below).
2. Open the "Tokeny" tab and choose "Generuj token".
3. Select the `InvoiceWrite` (submitting) and `InvoiceRead` (listing and
   downloading) permissions.
4. Copy the token. The app shows it only once.

Pass the token in an environment variable:

```bash
export KSEF_TOKEN='...'
```

## Environments

| `env`  | API                                  | Taxpayer app                   |
|--------|--------------------------------------|--------------------------------|
| `test` | `https://api-test.ksef.mf.gov.pl/v2` | https://ap-test.ksef.mf.gov.pl |
| `demo` | `https://api-demo.ksef.mf.gov.pl/v2` | https://ap-demo.ksef.mf.gov.pl |
| `prod` | `https://api.ksef.mf.gov.pl/v2`      | https://ap.ksef.mf.gov.pl      |

The test environment is shared by all integrators, so real invoices should
not be sent there.

## Configuration

The file `~/.config/ksef.conf`, in `key=value` format. The `KSEF_CONFIG`
variable points to a different path. The same paths apply on every system.

```
env=test            # test | demo | prod
nip=1234567890      # the NIP the program logs in as
ksef_token=         # leave empty and set KSEF_TOKEN
# cache_dir=~/.cache/ksef
```

| Key               | Meaning                                                        |
|-------------------|----------------------------------------------------------------|
| `env`             | Environment. Defaults to `test`.                               |
| `nip`             | Your NIP. When unset, `send` uses the invoice's seller NIP.    |
| `ksef_token`      | KSeF token. The `KSEF_TOKEN` variable always takes precedence. |
| `cache_dir`       | Directory for KSeF's public keys. Defaults to `~/.cache/ksef`. |
| `convert_options` | Options that `ksef convert` always starts with.                |

Only KSeF's public keys are written to disk, refreshed once a day. Every
command logs in from scratch.

## Commands

### `ksef init [test|demo|prod]`

Creates the config file, readable by its owner only. It does not overwrite
an existing file. If `KSEF_TOKEN` is set and contains a NIP, `init` writes
it into the file.

### `ksef selfcheck`

Checks encryption on this machine, fetches KSeF's public key and, when a
token is set, logs in.

```
[  OK  ] AES-256-CBC round-trip
[  OK  ] RSA-OAEP-SHA256 round-trip
[  OK  ] RSA-OAEP against KSeF's live published public key (env=test)
[  OK  ] KSeF login (env=test, nip=1234567891)
```

### `ksef convert [OPTIONS] INVOICE [OUT.xml]`

Converts an [Invoify](https://github.com/al1abb/invoify) export (JSON or
XML) into an FA(3) invoice. The result goes to standard output or to the
given file. It currently supports only VAT-exempt sellers (art. 113).
Invoices with tax, a discount or a shipping cost are refused.

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

Options go before the files. They cover values Invoify has no fields for:

| Option               | Meaning                                                  |
|----------------------|----------------------------------------------------------|
| `--sale-date DATE`   | Sale date, when it differs from the invoice date.        |
|                      | `prev-month-end`: the last day of the previous month.    |
| `--paid`             | Marks the invoice as paid today.                         |
| `--paid-date DATE`   | Marks the invoice as paid on the given day.              |
| `--due-date DATE`    | Payment due date, instead of the invoice's.              |
| `--place TEXT`       | Place of issue.                                          |
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

### `ksef send INVOICE.xml`

Submits one invoice and waits up to about a minute until KSeF accepts it.

```
Sending invoice.xml to KSeF (env=test)...
[  OK  ] accepted, KSeF number 1234567890-20260201-0123456789AB-CD
  invoice: FV/2026/02/1, issued 2026-02-01, sale 2026-01-31
  seller:  Jan Kowalski, NIP 1234567890
  buyer:   Beispiel GmbH, VAT DE123456789
  total:   1200.00 EUR at 4.2137
  payment: due 2026-02-15
  UPO:     invoice.upo.xml
  verify:  https://qr-test.ksef.mf.gov.pl/invoice/1234567890/01-02-2026/...
```

- A file that is not an FA invoice is refused before connecting to KSeF.
- The form code (e.g. `FA (3)`, `1-0E`) comes from the invoice itself.
- The UPO is saved next to the invoice as `INVOICE.upo.xml`.
- If the UPO file already exists, `send` refuses to submit. The invoice
  looks already sent.
- If KSeF considers the invoice a duplicate, the message gives the KSeF
  number of the original.
- If KSeF does not finish within a minute, or the status cannot be read,
  `send` gives the session and invoice numbers. Do not submit it again. Check
  later with `ksef list`.
- If the connection drops at the moment of submission, the invoice may have
  reached KSeF. Check `ksef list` before submitting it again.

### `ksef list [YEAR]`

Shows the invoices issued in the given year with you as the seller, oldest
first: date, invoice number, gross amount, buyer, mode, KSeF number. Without
an argument it takes the current year.

```
Invoices issued by 1234567890 in 2026 (env=test): 2
DATE        NUMBER              GROSS  BUYER          MODE     KSEF NUMBER
2026-01-02  FV/2026/01/1  1200.00 EUR  Beispiel GmbH  Online   1234567890-20260102-0123456789AB-CD
2026-02-01  FV/2026/02/1  1200.00 EUR  Beispiel GmbH  Offline  1234567890-20260203-0123456789AB-EF
```

The mode is assigned by KSeF. `Offline` means the invoice reached KSeF later
than the day it is dated.

### `ksef pull KSEF_NUMBER [FILE]`

Downloads an invoice as stored in KSeF. It prints it to standard output or
saves it to the given file. In a terminal it colours the XML syntax and shows a
summary of the invoice below. Redirected output is always the XML alone,
unchanged.

### `ksef version`

Prints the program's version.

Every command exits with code 0 on success, 1 on an error and 2 on an
unknown command. Messages are coloured only in a terminal. The `NO_COLOR`
variable switches colours off.

## What the tool does not do

- It does not create invoices from scratch. `convert` only turns an Invoify
  export into one, and only for VAT-exempt sellers.
- It does not check an invoice against the XSD schema. KSeF does that on
  submission.
- It does not show purchase invoices, only issued ones.
- It does not support certificate login (XAdES) or batch sessions.

## Development

All code lives in `cmd/ksef/` as a single `main` package and uses only the
Go standard library.

An invoice has one shared model (`invoice.go`) that checks the data and
produces the FA(3) XML. Invoify is just one reader that fills that model
(`invoify.go`). A new field is added once, in the model and the template, and
every reader can use it.

```bash
go test ./...
go run ./cmd/ksef selfcheck
```

The tests run the full login, submission, listing and download flow against
a local, fake KSeF server.
