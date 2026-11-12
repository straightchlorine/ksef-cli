# ksef-cli

Narzędzie wiersza poleceń do wysyłania faktur do KSeF (Krajowy System
e-Faktur), API 2.0.

## Instalacja

Potrzebny jest Go 1.26 lub nowszy.

```bash
go install codeberg.org/piotrkrzysztof/ksef-cli/cmd/ksef@latest
```

Polecenie instaluje program `ksef` w `~/go/bin`.

## Token KSeF

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

## Konfiguracja

Plik `~/.config/ksef.conf` w formacie `klucz=wartość`. Inną ścieżkę wskazuje
zmienna `KSEF_CONFIG`. Te same ścieżki obowiązują na każdym systemie.

```
env=test            # test | demo | prod
nip=1234567890      # NIP, na który program ma się zalogować
ksef_token=         # zostaw puste i ustaw KSEF_TOKEN
# cache_dir=~/.cache/ksef
```

| Klucz        | Opis                                                             |
|--------------|------------------------------------------------------------------|
| `env`        | Środowisko. Domyślnie `test`.                                    |
| `nip`        | Twój NIP. Gdy go brak, `send` bierze NIP sprzedawcy z faktury.   |
| `ksef_token` | Token KSeF. Zmienna `KSEF_TOKEN` zawsze ma pierwszeństwo.        |
| `cache_dir`  | Katalog na klucze publiczne KSeF. Domyślnie `~/.cache/ksef`.     |
| `convert_options` | Opcje, od których zawsze zaczyna `ksef convert`.               |

Na dysk trafiają tylko klucze publiczne KSeF, odświeżane raz na dobę. Każde polecenie
loguje się od nowa.

## Polecenia

### `ksef init [test|demo|prod]`

Tworzy plik konfiguracyjny z uprawnieniami tylko dla właściciela. Nie
nadpisuje istniejącego pliku. Jeśli `KSEF_TOKEN` jest ustawiony i zawiera
NIP, `init` wpisuje go do pliku.

### `ksef selfcheck`

Sprawdza szyfrowanie na tym komputerze, pobiera klucz publiczny KSeF i, gdy
token jest ustawiony, loguje się.

```
[  OK  ] AES-256-CBC round-trip
[  OK  ] RSA-OAEP-SHA256 round-trip
[  OK  ] RSA-OAEP against KSeF's live published public key (env=test)
[  OK  ] KSeF login (env=test, nip=1234567891)
```

### `ksef convert [OPCJE] FAKTURA [WYNIK.xml]`

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
| odbiorca | `NIP`      | Polski NIP nabywcy.                              |
| odbiorca | `VAT`      | Numer VAT UE z prefiksem kraju, np. `DE123...`.  |
| odbiorca | `NrID`     | Inny zagraniczny identyfikator.                  |
| odbiorca | `KodKraju` | Dwuliterowy kod kraju nabywcy.                   |

Pole `VAT` może też nazywać się `NrVatUE`, `VAT UE` albo `VAT ID`. Bez
żadnego identyfikatora nabywca jest oznaczany jako podmiot bez
identyfikatora. Kraj nabywcy to kod dwuliterowy (`DE`) albo nazwa po polsku
lub angielsku (`Niemcy`, `Germany`).

Opcje podaje się przed plikami. Dotyczą wartości, na które Invoify nie ma
pól:

| Opcja                | Znaczenie                                               |
|----------------------|---------------------------------------------------------|
| `--sale-date DATA`   | Data sprzedaży, gdy różni się od daty wystawienia.      |
|                      | `prev-month-end`: ostatni dzień poprzedniego miesiąca.  |
| `--paid`             | Oznacza fakturę jako zapłaconą dziś.                    |
| `--paid-date DATA`   | Oznacza fakturę jako zapłaconą w podanym dniu.          |
| `--due-date DATA`    | Termin płatności zamiast tego z faktury.                |
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
poleceń mają pierwszeństwo:

```
convert_options=--sale-date prev-month-end --paid
```

Po konwersji `convert` pokazuje podsumowanie faktury: numer, daty, strony,
pozycje, kwotę z kursem i płatność. Wiersz `not set` wymienia opcjonalne
wartości, których nie ustawiono, a `ignored` pola dodatkowe z Invoify, których
`convert` nie czyta.

Przejrzyj wynikowy XML przed wysyłką. Za treść faktury odpowiada jej
wystawca.

### `ksef send FAKTURA.xml`

Wysyła jedną fakturę i czeka do około minuty, aż KSeF ją przyjmie.

```
Sending faktura.xml to KSeF (env=test)...
[  OK  ] accepted, KSeF number 1234567890-20260201-0123456789AB-CD
  invoice: FV/2026/02/1, issued 2026-02-01, sale 2026-01-31
  seller:  Jan Kowalski, NIP 1234567890
  buyer:   Beispiel GmbH, VAT DE123456789
  total:   1200.00 EUR at 4.2137
  payment: due 2026-02-15
  UPO:     faktura.upo.xml
  verify:  https://qr-test.ksef.mf.gov.pl/invoice/1234567890/01-02-2026/...
```

- Plik, który nie jest fakturą FA, jest odrzucany przed połączeniem z KSeF.
- Kod formularza (np. `FA (3)`, `1-0E`) pochodzi z samej faktury.
- UPO trafia obok faktury jako `FAKTURA.upo.xml`.
- Jeśli plik UPO już istnieje, `send` odmawia wysyłki. Faktura wygląda na
  wysłaną.
- Jeśli KSeF uzna fakturę za duplikat, komunikat podaje numer KSeF oryginału.
- Jeśli KSeF nie skończy w minutę, `send` podaje numery sesji i faktury.
  Nie wysyłaj jej ponownie. Sprawdź później przez `ksef list`.

### `ksef list [ROK]`

Pokazuje faktury wystawione w danym roku, w których jesteś sprzedawcą, od
najstarszej: data, numer faktury, kwota brutto, nabywca, tryb, numer KSeF. Bez
argumentu bierze bieżący rok.

```
Invoices issued by 1234567890 in 2026 (env=test): 2
DATE        NUMBER              GROSS  BUYER          MODE     KSEF NUMBER
2026-01-02  FV/2026/01/1  1200.00 EUR  Beispiel GmbH  Online   1234567890-20260102-0123456789AB-CD
2026-02-01  FV/2026/02/1  1200.00 EUR  Beispiel GmbH  Offline  1234567890-20260203-0123456789AB-EF
```

Tryb nadaje KSeF. `Offline` oznacza, że faktura dotarła do KSeF później niż
w dniu, którym jest datowana.

### `ksef pull NUMER_KSEF [PLIK]`

Pobiera fakturę w postaci zapisanej w KSeF. Wypisuje ją na wyjście albo zapisuje
do podanego pliku. W terminalu koloruje składnię XML i pokazuje pod spodem
podsumowanie faktury. Przekierowane wyjście to zawsze sam, niezmieniony XML.

### `ksef version`

Wypisuje wersję programu.

Każde polecenie kończy się kodem 0 przy powodzeniu, 1 przy błędzie i 2 przy
nieznanym poleceniu. Komunikaty są kolorowane tylko w terminalu. Kolory
wyłącza zmienna `NO_COLOR`.

## Czego narzędzie nie robi

- Nie tworzy faktur od zera. `convert` zamienia tylko eksport z Invoify,
  i tylko dla sprzedawców zwolnionych z VAT.
- Nie sprawdza faktury względem schematu XSD. Robi to KSeF przy wysyłce.
- Nie pokazuje faktur zakupowych, tylko wystawione.
- Nie obsługuje logowania certyfikatem (XAdES) ani sesji wsadowych.

## Rozwój

Cały kod leży w `cmd/ksef/` jako jeden pakiet `main` i korzysta wyłącznie
z biblioteki standardowej Go.

```bash
go test ./...
go run ./cmd/ksef selfcheck
```

Testy uruchamiają pełny przebieg logowania, wysyłki, listy i pobierania na
lokalnym, udawanym serwerze KSeF.
