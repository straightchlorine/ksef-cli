package main

import (
	"bufio"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
)

type Config struct {
	Env      string // test | demo | prod
	NIP      string
	Token    string // $KSEF_TOKEN overrides the config file
	CacheDir string
}

// xdgPath returns $envVar/suffix, or ~/fallback/suffix when the variable is unset
func xdgPath(envVar, fallback, suffix string) string {
	dir := os.Getenv(envVar)
	if dir == "" {
		home, err := os.UserHomeDir()
		if err != nil {
			return ""
		}
		dir = filepath.Join(home, fallback)
	}
	return filepath.Join(dir, suffix)
}

// configPath returns $KSEF_CONFIG if set (explicit), else the default location.
func configPath() (path string, explicit bool) {
	if p := os.Getenv("KSEF_CONFIG"); p != "" {
		return p, true
	}
	return xdgPath("XDG_CONFIG_HOME", ".config", "ksef.conf"), false
}

// loadConfig reads $KSEF_CONFIG if set (it must exist - the user asked for that exact file),
// else $XDG_CONFIG_HOME/ksef.conf (default ~/.config/ksef.conf).
// At the default path only a missing file is fine; an existing-but-unreadable
// config is an error.
func loadConfig() (*Config, error) {
	cfg := &Config{Env: "test"}

	path, explicit := configPath()
	if path != "" {
		f, err := os.Open(path)
		switch {
		case err == nil:
			parseErr := parseConfig(f, cfg)
			f.Close()
			if parseErr != nil {
				return nil, fmt.Errorf("%s: %w", path, parseErr)
			}
		case explicit || !errors.Is(err, fs.ErrNotExist):
			return nil, err
		}
	}

	if t := os.Getenv("KSEF_TOKEN"); t != "" {
		cfg.Token = t
	}
	if cfg.CacheDir == "" {
		cfg.CacheDir = xdgPath("XDG_CACHE_HOME", ".cache", "ksef")
		if cfg.CacheDir == "" {
			return nil, errors.New("cache_dir not set and home directory unknown")
		}
	}
	switch cfg.Env {
	case "test", "demo", "prod":
	default:
		return nil, fmt.Errorf("env must be test|demo|prod, got %q", cfg.Env)
	}
	return cfg, nil
}

// parseConfig reads key=value shell-style lines.
func parseConfig(r io.Reader, cfg *Config) error {
	sc := bufio.NewScanner(r)
	for sc.Scan() {
		line := strings.TrimSpace(sc.Text())
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		key, rawVal, ok := strings.Cut(line, "=")
		if !ok {
			return fmt.Errorf("not a key=value line: %q", line)
		}
		val := parseValue(rawVal)
		switch strings.TrimSpace(key) {
		case "env":
			cfg.Env = val
		case "nip":
			cfg.NIP = val
		case "ksef_token":
			cfg.Token = val
		case "cache_dir":
			cfg.CacheDir = expandHome(val)
		}
	}
	return sc.Err()
}

// parseValue reads a value the way the shell would.
func parseValue(s string) string {
	if q := strings.TrimLeft(s, " \t"); len(q) != len(s) && strings.HasPrefix(q, "#") {
		return ""
	}
	s = strings.TrimSpace(s)
	if len(s) >= 2 && (s[0] == '"' || s[0] == '\'') {
		if end := strings.IndexByte(s[1:], s[0]); end >= 0 {
			return s[1 : 1+end]
		}
	}
	for i := 1; i < len(s); i++ {
		if s[i] == '#' && (s[i-1] == ' ' || s[i-1] == '\t') {
			s = s[:i]
			break
		}
	}
	return strings.TrimSpace(s)
}

func expandHome(path string) string {
	if path == "~" || strings.HasPrefix(path, "~/") {
		if home, err := os.UserHomeDir(); err == nil {
			return filepath.Join(home, path[1:])
		}
	}
	return path
}
