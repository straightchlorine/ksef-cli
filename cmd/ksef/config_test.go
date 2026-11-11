package main

import (
	"os"
	"path/filepath"
	"testing"
)

func writeConf(t *testing.T, content string) {
	t.Helper()
	path := filepath.Join(t.TempDir(), "ksef.conf")
	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
		t.Fatal(err)
	}
	t.Setenv("KSEF_CONFIG", path)
	t.Setenv("KSEF_TOKEN", "")
}

func TestLoadConfig(t *testing.T) {
	writeConf(t, `
# comment
env=demo
nip = 1234567890
ksef_token="secret"
cache_dir='/tmp/ksef-test-cache'
convert_options=--sale-date prev-month-end --paid
unknown_key_from_other_tool=ignored
`)
	cfg, err := loadConfig()
	if err != nil {
		t.Fatal(err)
	}
	if cfg.Env != "demo" || cfg.NIP != "1234567890" || cfg.Token != "secret" ||
		cfg.ConvertOptions != "--sale-date prev-month-end --paid" ||
		cfg.CacheDir != "/tmp/ksef-test-cache" {
		t.Errorf("got %+v", cfg)
	}
}

func TestParseValue(t *testing.T) {
	for in, want := range map[string]string{
		"test":               "test",
		"test   # c":         "test",
		"  # only a comment": "",
		"#literal":           "#literal",
		"te#st":              "te#st",
		`"te # st" # c`:      "te # st",
		"'x'":                "x",
		"a=b":                "a=b",
		"":                   "",
	} {
		if got := parseValue(in); got != want {
			t.Errorf("parseValue(%q) = %q, want %q", in, got, want)
		}
	}
}

// The README's own config example must parse - inline comments included.
func TestReadmeExampleParses(t *testing.T) {
	writeConf(t, `
env=test          # test | demo | prod
nip=1234567890
ksef_token=       # zostaw puste i ustaw $KSEF_TOKEN w środowisku
cache_dir=~/.cache/ksef
`)
	cfg, err := loadConfig()
	if err != nil {
		t.Fatal(err)
	}
	if cfg.Env != "test" || cfg.NIP != "1234567890" || cfg.Token != "" {
		t.Errorf("got %+v", cfg)
	}
}

func TestTokenEnvOverride(t *testing.T) {
	writeConf(t, "ksef_token=from-file\n")
	t.Setenv("KSEF_TOKEN", "from-env")
	cfg, err := loadConfig()
	if err != nil {
		t.Fatal(err)
	}
	if cfg.Token != "from-env" {
		t.Errorf("got %q", cfg.Token)
	}
}

func TestBadEnvRejected(t *testing.T) {
	writeConf(t, "env=production\n")
	if _, err := loadConfig(); err == nil {
		t.Error("env=production should be rejected")
	}
}

func TestDefaults(t *testing.T) {
	// Point config lookup at an empty dir so the developer's real
	// ~/.config/ksef.conf can't leak into the test.
	t.Setenv("KSEF_CONFIG", "")
	t.Setenv("KSEF_TOKEN", "")
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	cfg, err := loadConfig()
	if err != nil {
		t.Fatal(err)
	}
	if cfg.Env != "test" || cfg.CacheDir == "" {
		t.Errorf("got %+v", cfg)
	}
}

func TestMissingExplicitConfigFails(t *testing.T) {
	t.Setenv("KSEF_CONFIG", filepath.Join(t.TempDir(), "nope.conf"))
	if _, err := loadConfig(); err == nil {
		t.Error("missing $KSEF_CONFIG file should be an error")
	}
}
