---
title: "The ksef.conf file and environment variables"
description: "The ~/.config/ksef.conf file in key=value format: env, nip, ksef_token, cache_dir and convert_options. KSEF_CONFIG and KSEF_TOKEN variables, what goes to disk."
---

# Configuration

The file `~/.config/ksef.conf`, in `key=value` format. The `KSEF_CONFIG`
variable points to a different path. The same paths apply on every system.

```
env=test            # test | demo | prod
nip=1234567890      # the NIP the program logs in as
ksef_token=         # leave empty and set KSEF_TOKEN
# cache_dir=~/.cache/ksef
```

| Key               | Meaning                                                        |
|-------------------|----------------------------------------------------------------|
| `env`             | Environment. Defaults to `test`.                               |
| `nip`             | Your NIP. When unset, `send` uses the invoice's seller NIP.    |
| `ksef_token`      | KSeF token. The `KSEF_TOKEN` variable always takes precedence. |
| `cache_dir`       | Directory for KSeF's public keys. Defaults to `~/.cache/ksef`. |
| `convert_options` | Options that `ksef convert` always starts with.                |

Only KSeF's public keys are written to disk, refreshed once a day. Every
command logs in from scratch.
