---
title: "KSeF token and the test, demo, prod environments"
description: "How to generate a KSeF token in the taxpayer app, which permissions to select and how to pass it in the KSEF_TOKEN variable. Addresses of test, demo and prod."
---

# KSeF token

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
