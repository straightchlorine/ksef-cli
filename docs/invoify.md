---
title: "Konwersja faktury z Invoify na XML FA(3)"
description: "ksef convert zamienia eksport JSON lub XML z Invoify na fakturę FA(3) sprzedawcy zwolnionego z VAT. Pola dodatkowe, opcje, kurs średni NBP i opcje w ksef.conf."
---

# Faktury z Invoify

## `ksef convert [OPCJE] FAKTURA [WYNIK.xml]`

Zamienia eksport z [Invoify](https://github.com/al1abb/invoify) (JSON albo
XML) na fakturę FA(3). Wynik trafia na wyjście albo do podanego pliku.
Obecnie obsługuje tylko sprzedawców zwolnionych z VAT (art. 113).
Faktury z podatkiem, rabatem albo kosztem wysyłki są odrzucane.

```bash
ksef convert faktura.json faktura.xml
ksef send faktura.xml
```

Invoify nie ma pól na identyfikatory podatkowe, więc `convert` czyta je
z pól dodatkowych (custom inputs):

| Strona   | Pole       | Znaczenie                                        |
|----------|------------|--------------------------------------------------|
| nadawca  | `NIP`      | NIP sprzedawcy. Wymagany.                        |
| nadawca  | `REGON`    | REGON, trafia do stopki.                         |
| nadawca  | `SWIFT`    | Kod SWIFT rachunku (także jako `BIC`).           |
| odbiorca | `NIP`      | Polski NIP nabywcy.                              |
| odbiorca | `VAT`      | Numer VAT UE z prefiksem kraju, np. `DE123...`.  |
| odbiorca | `NrID`     | Inny zagraniczny identyfikator.                  |
| odbiorca | `KodKraju` | Dwuliterowy kod kraju nabywcy.                   |

Pole `VAT` może też nazywać się `NrVatUE`, `VAT UE` albo `VAT ID`. Bez
żadnego identyfikatora nabywca jest oznaczany jako podmiot bez
identyfikatora. Kraj nabywcy to kod dwuliterowy (`DE`) albo nazwa po polsku
lub angielsku (`Niemcy`, `Germany`).

NIP jest sprawdzany: 10 cyfr i cyfra kontrolna. Zagraniczny numer wpisany
w polu `NIP` jest odrzucany. Numer VAT spoza UE (np. `GB...`) trafia na fakturę
jako inny identyfikator zagraniczny.

Z faktury Invoify przenoszone są też: opis pozycji, warunki płatności, numer
zamówienia oraz każde inne pole dodatkowe. Trafiają na fakturę jako dodatkowe
opisy (pary klucz i wartość), a numer zamówienia do warunków transakcji.

Opcje podaje się przed plikami. Dotyczą wartości, na które Invoify nie ma
pól:

| Opcja                | Znaczenie                                               |
|----------------------|---------------------------------------------------------|
| `--sale-date DATA`   | Data sprzedaży, gdy różni się od daty wystawienia.      |
|                      | `prev-month-end`: ostatni dzień poprzedniego miesiąca.  |
| `--paid`             | Oznacza fakturę jako zapłaconą dziś.                    |
| `--paid-date DATA`   | Oznacza fakturę jako zapłaconą w podanym dniu.          |
| `--due-date DATA`    | Termin płatności zamiast tego z faktury.                |
| `--place TEKST`      | Miejsce wystawienia faktury.                            |
| `--rate KURS`        | Kurs waluty zamiast kursu NBP.                          |
| `--rate-date DATA`   | Kurs średni NBP z tego dnia albo ostatni przed nim.     |
| `--buyer-country XX` | Kod kraju nabywcy, gdy nazwa nie jest rozpoznana.       |
| `--buyer-id TEKST`   | Zagraniczny identyfikator nabywcy.                      |
| `--unit TEKST`       | Jednostka miary pozycji. Domyślnie `m-c` (miesiąc).     |
| `--exemption TEKST`  | Podstawa zwolnienia. Domyślnie art. 113 ust. 1.         |

Faktura w walucie obcej dostaje kurs średni NBP (tabela A, publiczne API)
z ostatniego dnia roboczego przed datą wystawienia. `--rate-date` wybiera
inny dzień, a `--rate` podaje kurs wprost.

Przykład faktury w euro dla kontrahenta z Niemiec:

```bash
ksef convert --sale-date 2026-10-31 faktura.json faktura.xml
```

Regularnie używane opcje można wpisać raz do `ksef.conf`. Parametry z wiersza
poleceń mają pierwszeństwo. Wartości ze spacją ujmij w cudzysłów, np.
`--place "Nowy Sącz"`:

```
convert_options=--sale-date prev-month-end --paid
```

Po konwersji `convert` pokazuje podsumowanie faktury: numer, daty, strony,
pozycje, kwotę z kursem i płatność. Wiersz `extra` wymienia dodatkowe
opisy, a `not set` opcjonalne wartości, których nie ustawiono.

Przejrzyj wynikowy XML przed wysyłką. Za treść faktury odpowiada jej
wystawca.
