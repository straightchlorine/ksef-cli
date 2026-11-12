---
title: "O projekcie ksef-cli, ograniczenia i licencja"
description: "Czego ksef-cli nie robi, jak ułożony jest kod i jak uruchomić testy na lokalnym, udawanym serwerze KSeF. Licencja MIT, autor oraz repozytorium na Codeberg."
---

# O projekcie

## Czego narzędzie nie robi

- Nie tworzy faktur od zera. `convert` zamienia tylko eksport z Invoify,
  i tylko dla sprzedawców zwolnionych z VAT.
- Nie sprawdza faktury względem schematu XSD. Robi to KSeF przy wysyłce.
- Nie pokazuje faktur zakupowych, tylko wystawione.
- Nie obsługuje logowania certyfikatem (XAdES) ani sesji wsadowych.

## Rozwój

Cały kod leży w `cmd/ksef/` jako jeden pakiet `main` i korzysta wyłącznie
z biblioteki standardowej Go.

Faktura ma jeden wspólny model (`invoice.go`), który sprawdza dane i tworzy XML
FA(3). Invoify jest tylko jednym czytnikiem, który ten model wypełnia
(`invoify.go`). Nowe pole dodaje się raz w modelu i szablonie, a każdy czytnik
może z niego korzystać.

```bash
go test ./...
go run ./cmd/ksef selfcheck
```

Testy uruchamiają pełny przebieg logowania, wysyłki, listy i pobierania na
lokalnym, udawanym serwerze KSeF.

## Licencja

Licencja MIT. Szczegóły w pliku [LICENSE](https://codeberg.org/piotrkrzysztof/ksef-cli/src/branch/main/LICENSE).

## Autor

Piotr Krzysztof Lis, [piotrkrzysztof.dev](https://piotrkrzysztof.dev). Kod źródłowy jest na [Codeberg](https://codeberg.org/piotrkrzysztof/ksef-cli), a jego kopia na [GitHubie](https://github.com/straightchlorine/ksef-cli).
