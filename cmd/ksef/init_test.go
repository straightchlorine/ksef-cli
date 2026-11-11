package main

import (
	"path/filepath"
	"testing"
)

// What init writes must load back, and init must never overwrite.
func TestInit(t *testing.T) {
	t.Setenv("KSEF_CONFIG", filepath.Join(t.TempDir(), "sub", "ksef.conf"))
	t.Setenv("KSEF_TOKEN", "20260101-EC-AAAA|nip-1234567890|deadbeef")

	if err := cmdInit([]string{"production"}); err == nil {
		t.Error("unknown env should be rejected")
	}
	if err := cmdInit([]string{"demo"}); err != nil {
		t.Fatal(err)
	}
	cfg, err := loadConfig()
	if err != nil {
		t.Fatal(err)
	}
	if cfg.Env != "demo" || cfg.NIP != "1234567890" {
		t.Errorf("got %+v", cfg)
	}
	if err := cmdInit(nil); err == nil {
		t.Error("init must not overwrite an existing config")
	}
}
