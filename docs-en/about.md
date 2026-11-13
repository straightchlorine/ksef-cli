---
title: "About ksef-cli, its limits, tests and license"
description: "What ksef-cli does not do, how the code is laid out and how to run the tests against a local, fake KSeF server. MIT license, author and the Codeberg repository."
---

# About

## What the tool does not do

- It does not create invoices from scratch. `convert` only turns an Invoify
  export into one: VAT-exempt, or with a single rate of 23, 8 or 5%.
- It does not check an invoice against the XSD schema. KSeF does that on
  submission.
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

## License

MIT License. See [LICENSE](https://codeberg.org/piotrkrzysztof/ksef-cli/src/branch/main/LICENSE) for details.

## Author

Piotr Krzysztof Lis, [piotrkrzysztof.dev](https://piotrkrzysztof.dev). The source code is on [Codeberg](https://codeberg.org/piotrkrzysztof/ksef-cli), with a mirror on [GitHub](https://github.com/straightchlorine/ksef-cli).
