---
title: "Submit invoices to KSeF from the command line"
description: "ksef-cli submits a ready FA(3) invoice XML to KSeF from the command line: token login, encryption, the UPO receipt, the KSeF number and a verification link."
---

# ksef-cli

A command-line tool for submitting invoices to KSeF (Krajowy System
e-Faktur, Poland's national e-invoicing system), API 2.0.

You provide a ready FA(3) invoice XML file. `ksef` then logs in with a token,
encrypts the invoice, submits it and waits until KSeF accepts it.

At the end it saves the UPO receipt next to the invoice and prints the KSeF
number together with a public verification link.

The tool does not create invoices from scratch!

## Features

- Submits a ready FA(3) XML and saves the UPO as `invoice.upo.xml`
- Prints the KSeF number and the invoice's public link
- Lists the invoices issued in a given year and downloads them from KSeF
- Works with the test, demo and production environments
- Converts an [Invoify](https://github.com/al1abb/invoify) invoice (JSON or XML
  export) into an FA(3) XML: VAT-exempt (art. 113), or with a rate of 23, 8 or 5%
- For foreign-currency invoices, fetches the mid rate from NBP's public API

## Installation

You need Go 1.26 or newer.

```bash
go install codeberg.org/piotrkrzysztof/ksef-cli/cmd/ksef@latest
```

The command installs the `ksef` program in `~/go/bin`.

## Getting started

You need Go 1.26 or newer and a token from the KSeF taxpayer app (Aplikacja
Podatnika KSeF, "Tokeny" tab).

```bash
export KSEF_TOKEN='...'    # token from the taxpayer app
ksef init test             # creates ~/.config/ksef.conf
ksef selfcheck             # checks encryption and login
ksef send invoice.xml      # submits the invoice, saves invoice.upo.xml
ksef list                  # invoices issued this year
ksef pull KSEF_NUMBER      # downloads an invoice as stored in KSeF
```

Start with the test environment. Switch to production only after you have
verified on the test instance that everything is correct.

Keep in mind that every entry in production KSeF has legal consequences.
According to a [statement by the Ministry of Finance](https://www.gov.pl/web/finanse/przedluzenie-odroczenia-kar-za-bledy-w-stosowaniu-ksef-do-konca-2027-r),
financial penalties for mistakes in using the system will not be enforced
until the end of 2027. Issuing invoices in KSeF is nevertheless [mandatory from 1 January 2027](https://ksef.podatki.gov.pl/informacje-ogolne-ksef-20/zakres-obowiazkowego-ksef)
for all businesses, regardless of their turnover.
