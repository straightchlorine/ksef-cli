---
title: "Polecenia init, selfcheck, send, list i pull"
description: "Polecenia ksef: init, selfcheck, send, status, list, pull i version. Co wypisują, kiedy send odmawia wysyłki, co znaczy tryb Offline na liście i kody wyjścia."
---

# Polecenia

## `ksef init [test|demo|prod]`

Tworzy plik konfiguracyjny z uprawnieniami tylko dla właściciela. Nie
nadpisuje istniejącego pliku. Jeśli `KSEF_TOKEN` jest ustawiony i zawiera
NIP, `init` wpisuje go do pliku.

## `ksef selfcheck`

Sprawdza szyfrowanie na tym komputerze, pobiera klucz publiczny KSeF i, gdy
token jest ustawiony, loguje się.

```
[  OK  ] AES-256-CBC round-trip
[  OK  ] RSA-OAEP-SHA256 round-trip
[  OK  ] RSA-OAEP against KSeF's live published public key (env=test)
[  OK  ] KSeF login (env=test, nip=1234567891)
```

## `ksef send FAKTURA.xml`

Wysyła jedną fakturę i czeka do około minuty, aż KSeF ją przyjmie.

```
Sending faktura.xml to KSeF (env=test)...
[  OK  ] accepted, KSeF number 1234567890-20260201-0123456789AB-CD
  invoice: FV/2026/02/1, issued 2026-02-01, sale 2026-01-31
  seller:  Jan Kowalski, NIP 1234567890
  buyer:   Beispiel GmbH, VAT DE123456789
  total:   1200.00 EUR at 4.2137
  payment: due 2026-02-15
  session: 20260201-SO-0123456789-0123456789-AB
  UPO:     faktura.upo.xml
  verify:  https://qr-test.ksef.mf.gov.pl/invoice/1234567890/01-02-2026/...
```

- Plik, który nie jest fakturą FA, jest odrzucany przed połączeniem z KSeF.
- Kod formularza (np. `FA (3)`, `1-0E`) pochodzi z samej faktury.
- UPO trafia obok faktury jako `FAKTURA.upo.xml`.
- Numer sesji z tego wyniku przyjmuje `ksef status`.
- Jeśli plik UPO już istnieje, `send` odmawia wysyłki. Faktura wygląda na
  wysłaną.
- Jeśli KSeF uzna fakturę za duplikat, komunikat podaje numer KSeF oryginału.
- Jeśli KSeF nie skończy w minutę albo nie da się odczytać statusu, `send`
  podaje numer sesji. Nie wysyłaj jej ponownie. Sprawdź później
  przez `ksef status SESJA`.
- Jeśli połączenie zerwie się w chwili wysyłki, faktura mogła dotrzeć do KSeF.
  Zanim wyślesz ją ponownie, sprawdź `ksef status SESJA`.
- Gdy KSeF nie odpowiada przy logowaniu, każde polecenie próbuje do trzech razy.

## `ksef status SESJA [PLIK_UPO]`

Pokazuje, co stało się z fakturą, gdy `send` zakończył się przed odpowiedzią
KSeF. Numer sesji podaje wtedy komunikat `send`.

```
[  OK  ] FV/2026/02/1 accepted, KSeF number 1234567890-20260201-0123456789AB-CD
  UPO:     faktura.upo.xml
```

- Faktura przyjęta: z podanym `PLIK_UPO` polecenie zapisuje UPO. Istniejącego
  pliku nie nadpisuje.
- Faktura w trakcie przetwarzania albo odrzucona: polecenie kończy się błędem
  i podaje powód.
- Sesja bez faktury: faktura nie dotarła do KSeF.

## `ksef list [--received] [ROK|RRRR-MM]`

Pokazuje faktury wystawione w danym roku albo miesiącu, w których jesteś sprzedawcą, od
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

Z opcją `--received` pokazuje faktury wystawione dla ciebie, a kolumna `BUYER`
zmienia się w `SELLER`. Miesiąc (`ksef list 2026-11`) to jedno zapytanie do KSeF,
rok to do czterech. KSeF przyjmuje 20 takich zapytań na godzinę.

## `ksef pull NUMER_KSEF [PLIK]`

Pobiera fakturę w postaci zapisanej w KSeF. Wypisuje ją na wyjście albo zapisuje
do podanego pliku. W terminalu koloruje składnię XML i pokazuje pod spodem
podsumowanie faktury. Przekierowane wyjście to zawsze sam, niezmieniony XML.

## `ksef version`

Wypisuje wersję programu.

Każde polecenie kończy się kodem 0 przy powodzeniu, 1 przy błędzie i 2 przy
nieznanym poleceniu. Komunikaty są kolorowane tylko w terminalu. Kolory
wyłącza zmienna `NO_COLOR`.
