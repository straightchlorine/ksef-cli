<h1 align="center">ksef-cli</h1>

<div align="center">

[Dokumentacja](docs/index.md) &middot; [English](docs/en.md) &middot; [Codeberg](https://codeberg.org/piotrkrzysztof/ksef-cli)

Wysyłanie faktur do KSeF z wiersza poleceń.

</div>

<br>

Należy podać gotowy plik XML faktury FA(3). `ksef` następnie loguje się tokenem,
szyfruje fakturę, wysyła ją i czeka, aż KSeF ją przyjmie.

Na koniec zapisuje UPO obok faktury i wypisuje numer KSeF wraz z publicznym linkiem
do weryfikacji.

Narzędzie nie tworzy faktur!

## Funkcje

- Wysyła gotowy XML FA(3) i zapisuje UPO jako `faktura.upo.xml`
- Wypisuje numer KSeF i publiczny link faktury
- Pokazuje faktury wystawione w danym roku i pobiera je z KSeF
- Działa ze środowiskiem testowym, demo i produkcyjnym
- Zamienia fakturę z [Invoify](https://github.com/al1abb/invoify) (eksport JSON
  lub XML) na XML FA(3) dla sprzedawców zwolnionych z VAT (art. 113)
- Dla faktur w walucie obcej pobiera kurs średni z publicznego API NBP

## Jak zacząć?

Potrzebny jest Go 1.26 lub nowszy oraz token z Aplikacji Podatnika KSeF
(zakładka „Tokeny").

```bash
go install codeberg.org/piotrkrzysztof/ksef-cli/cmd/ksef@latest

export KSEF_TOKEN='...'    # token z Aplikacji Podatnika
ksef init test             # tworzy ~/.config/ksef.conf
ksef selfcheck             # sprawdza szyfrowanie i logowanie
ksef send faktura.xml      # wysyła fakturę, zapisuje faktura.upo.xml
ksef list                  # faktury wystawione w tym roku
ksef pull NUMER_KSEF       # pobiera fakturę zapisaną w KSeF
```

Zacznij od środowiska testowego. Zmianę na środowisko produkcyjne powinno nastąpić
dopiero po zweryfikowaniu poprawności informacji na instancji testowej.

Pamiętać należy, że każdy wpis na produkcyjną witrynę KSeF ma konsekwencje prawne.
Zgodnie z [oświadczeniem Ministerstwa Finansów](https://www.gov.pl/web/finanse/przedluzenie-odroczenia-kar-za-bledy-w-stosowaniu-ksef-do-konca-2027-r?trk=public_post_comment-text),
do końca 2027 roku kary pieniężne za błędy w stosowaniu systemu nie będą jeszcze
egzekwowane. Publikowanie faktur w KSeF jest jednak [obowiązkowe od 1 stycznia 2027 roku](https://ksef.podatki.gov.pl/informacje-ogolne-ksef-20/zakres-obowiazkowego-ksef)
dla wszystkich przedsiębiorców - niezależnie od ich dochodów.

## Dokumentacja

Konfiguracja, token, opis poleceń i środowisk: [docs/index.md](docs/index.md).

## Licencja

Licencja MIT. Szczegóły w pliku [LICENSE](LICENSE).

---

<div align="center">

Bogu samemu - cześć i chwała na wieki wieków.

</div>
