---
title: "The init, selfcheck, send, list and pull commands"
description: "The ksef commands: init, selfcheck, send, list, pull and version. What they print, when send refuses to submit, what Offline mode means in the list, exit codes."
---

# Commands

## `ksef init [test|demo|prod]`

Creates the config file, readable by its owner only. It does not overwrite
an existing file. If `KSEF_TOKEN` is set and contains a NIP, `init` writes
it into the file.

## `ksef selfcheck`

Checks encryption on this machine, fetches KSeF's public key and, when a
token is set, logs in.

```
[  OK  ] AES-256-CBC round-trip
[  OK  ] RSA-OAEP-SHA256 round-trip
[  OK  ] RSA-OAEP against KSeF's live published public key (env=test)
[  OK  ] KSeF login (env=test, nip=1234567891)
```

## `ksef send INVOICE.xml`

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

## `ksef list [YEAR]`

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

## `ksef pull KSEF_NUMBER [FILE]`

Downloads an invoice as stored in KSeF. It prints it to standard output or
saves it to the given file. In a terminal it colours the XML syntax and shows a
summary of the invoice below. Redirected output is always the XML alone,
unchanged.

## `ksef version`

Prints the program's version.

Every command exits with code 0 on success, 1 on an error and 2 on an
unknown command. Messages are coloured only in a terminal. The `NO_COLOR`
variable switches colours off.
