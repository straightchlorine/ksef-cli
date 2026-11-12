---
title: "Konfiguracja ksef.conf i zmienne środowiskowe"
description: "Plik ~/.config/ksef.conf w formacie klucz=wartość: env, nip, ksef_token, cache_dir i convert_options. Zmienne KSEF_CONFIG i KSEF_TOKEN, co trafia na dysk."
---

# Konfiguracja

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
