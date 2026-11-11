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
ma własną instancje i własne tokeny.

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

### `ksef send FAKTURA.xml`

Wysyła jedną fakturę i czeka do około minuty, aż KSeF ją przyjmie.

```
Sending faktura.xml to KSeF (env=test)...
[  OK  ] accepted, KSeF number 1234567890-20260201-0123456789AB-CD
  UPO:    faktura.upo.xml
  verify: https://qr-test.ksef.mf.gov.pl/invoice/1234567890/01-02-2026/...
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
najstarszej: data, numer faktury, kwota brutto, nabywca, numer KSeF. Bez
argumentu bierze bieżący rok.

### `ksef pull NUMER_KSEF [PLIK]`

Pobiera fakturę w postaci zapisanej w KSeF. Wypisuje ją na wyjście albo zapisuje
do podanego pliku.

### `ksef version`

Wypisuje wersję programu.

Każde polecenie kończy się kodem 0 przy powodzeniu, 1 przy błędzie i 2 przy
nieznanym poleceniu.

## Czego narzędzie nie robi

- Nie tworzy faktur. Wejściem jest gotowy XML FA(3).
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
