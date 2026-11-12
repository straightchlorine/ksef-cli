---
title: "Token KSeF i środowiska test, demo oraz prod"
description: "Jak wygenerować token KSeF w Aplikacji Podatnika, które uprawnienia zaznaczyć i jak przekazać go w zmiennej KSEF_TOKEN. Adresy środowisk test, demo i prod."
---

# Token KSeF

Token generowany jest raz, ręcznie, w Aplikacji Podatnika KSeF. Każde środowisko
ma własną instancję i własne tokeny.

1. Zaloguj się do Aplikacji Podatnika wybranego środowiska (adresy niżej).
2. Otwórz zakładkę „Tokeny" i wybierz „Generuj token".
3. Zaznacz uprawnienia `InvoiceWrite` (wysyłka) i `InvoiceRead` (lista
   i pobieranie).
4. Skopiuj token. Aplikacja pokazuje go tylko raz.

Token przekaż w zmiennej środowiskowej:

```bash
export KSEF_TOKEN='...'
```

## Środowiska

| `env`  | API                                  | Aplikacja Podatnika            |
|--------|--------------------------------------|--------------------------------|
| `test` | `https://api-test.ksef.mf.gov.pl/v2` | https://ap-test.ksef.mf.gov.pl |
| `demo` | `https://api-demo.ksef.mf.gov.pl/v2` | https://ap-demo.ksef.mf.gov.pl |
| `prod` | `https://api.ksef.mf.gov.pl/v2`      | https://ap.ksef.mf.gov.pl      |

Środowisko testowe jest wspólne dla wszystkich integratorów. Nie należy więc wysyłać
tam prawdziwych faktur.
