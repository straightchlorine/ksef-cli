package main

import (
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"regexp"
)

const confTemplate = `# ksef configuration, key=value

# KSeF environment: test | demo | prod
env=%s

# Your NIP (tax id) - the context you log in as
nip=%s

# KSeF token from the taxpayer app ("Tokeny" tab). Better left blank:
# export KSEF_TOKEN instead and the secret never touches this file.
ksef_token=

# Public-key cache
# cache_dir=~/.cache/ksef

# Options that ksef convert always starts with, e.g.
# convert_options=--sale-date prev-month-end --paid
`

var tokenNIP = regexp.MustCompile(`\|nip-(\d{10})\|`)

// cmdInit writes a starter config and never overwrites an existing one.
// nip is filled in from $KSEF_TOKEN when the token carries it.
func cmdInit(args []string) error {
	env := "test"
	if len(args) > 0 {
		env = args[0]
	}
	if _, ok := baseURLs[env]; !ok {
		return fmt.Errorf("env must be test|demo|prod, got %q", env)
	}
	path, _ := configPath()
	if path == "" {
		return errors.New("home directory unknown, set $KSEF_CONFIG")
	}
	token := os.Getenv("KSEF_TOKEN")
	nip := ""
	if m := tokenNIP.FindStringSubmatch(token); m != nil {
		nip = m[1]
	}

	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return err
	}
	f, err := os.OpenFile(path, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o600)
	if errors.Is(err, fs.ErrExist) {
		return fmt.Errorf("%s already exists, leaving it alone", path)
	}
	if err != nil {
		return err
	}
	if _, err := fmt.Fprintf(f, confTemplate, env, nip); err != nil {
		f.Close()
		return err
	}
	if err := f.Close(); err != nil {
		return err
	}

	fmt.Printf("%s wrote %s (%s)\n", paint(os.Stdout, green, "[  OK  ]"), path,
		envTag(os.Stdout, env))
	if nip == "" {
		fmt.Println("  - set nip= in it")
	}
	if token == "" {
		fmt.Println("  - export KSEF_TOKEN=<your KSeF token>")
	}
	fmt.Println("Then check the setup with: ksef selfcheck")
	return nil
}
