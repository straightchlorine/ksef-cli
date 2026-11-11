// ksef - submit invoices to KSeF e-invoicing system (API 2.0).
//
// Takes a ready FA(3) XML as input.
package main

import (
	"fmt"
	"os"
)

// Overridden at release via -ldflags "-X main.version=...".
var version = "dev"

const usage = `Usage: ksef init [test|demo|prod]
       ksef selfcheck
       ksef version
       ksef -h

Subcommands:
  init        Write a starter config, never overwriting an existing one.
              Environment defaults to test; nip is filled in from
              $KSEF_TOKEN when the token carries it.
  selfcheck   Checks the setup: AES/RSA crypto round-trip, KSeF's live
              published public key and, when a token is configured, a login.
  version     Print version.

Config (key=value; $KSEF_CONFIG if set, else $XDG_CONFIG_HOME/ksef.conf,
default ~/.config/ksef.conf - same paths on every OS):
  env         test | demo | prod (default: test)
  nip         your NIP (tax id) - the context you log in as
  ksef_token  KSeF token; $KSEF_TOKEN overrides it - leave the config value
              blank and export the secret instead
  cache_dir   public-key cache (default: ~/.cache/ksef)
`

func main() {
	if len(os.Args) < 2 || os.Args[1] == "-h" || os.Args[1] == "--help" {
		fmt.Print(usage)
		return
	}

	var err error
	switch cmd := os.Args[1]; cmd {
	case "init":
		err = cmdInit(os.Args[2:])
	case "selfcheck":
		err = cmdSelfcheck()
	case "version":
		fmt.Println(version)
	default:
		fmt.Fprintf(os.Stderr, "Unknown subcommand: %s\n\n%s", cmd, usage)
		os.Exit(2)
	}
	if err != nil {
		fmt.Fprintln(os.Stderr, "Error:", err)
		os.Exit(1)
	}
}
