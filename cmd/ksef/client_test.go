package main

import (
	"testing"
	"time"
)

func TestPickCert(t *testing.T) {
	day := 24 * time.Hour
	now := time.Date(2026, 8, 4, 12, 0, 0, 0, time.UTC)
	current := publicKeyCert{
		PublicKeyID: "current", Usage: []string{"SymmetricKeyEncryption"},
		ValidFrom: now.Add(-300 * day), ValidTo: now.Add(60 * day),
	}
	successor := publicKeyCert{
		PublicKeyID: "successor", Usage: []string{"SymmetricKeyEncryption"},
		ValidFrom: now.Add(30 * day), ValidTo: now.Add(400 * day),
	}
	expired := publicKeyCert{
		PublicKeyID: "expired", Usage: []string{"SymmetricKeyEncryption"},
		ValidFrom: now.Add(-700 * day), ValidTo: now.Add(-400 * day),
	}
	overlap := publicKeyCert{
		PublicKeyID: "overlap", Usage: []string{"SymmetricKeyEncryption"},
		ValidFrom: now.Add(-10 * day), ValidTo: now.Add(400 * day),
	}
	other := publicKeyCert{
		PublicKeyID: "other", Usage: []string{"KsefTokenEncryption"},
		ValidFrom: now.Add(-300 * day), ValidTo: now.Add(400 * day),
	}

	for name, tc := range map[string]struct {
		certs []publicKeyCert
		want  string // PublicKeyID, or "" for expected error
	}{
		"single valid":                  {[]publicKeyCert{current}, "current"},
		"future successor listed first": {[]publicKeyCert{successor, current}, "current"},
		"rotation overlap: latest wins": {[]publicKeyCert{current, overlap}, "overlap"},
		"expired filtered out":          {[]publicKeyCert{expired, current}, "current"},
		"usage respected":               {[]publicKeyCert{other, current}, "current"},
		"none valid":                    {[]publicKeyCert{expired, successor}, ""},
		"wrong usage only":              {[]publicKeyCert{other}, ""},
	} {
		got, err := pickCert(tc.certs, "SymmetricKeyEncryption", now)
		if tc.want == "" {
			if err == nil {
				t.Errorf("%s: expected error, got %s", name, got.PublicKeyID)
			}
			continue
		}
		if err != nil {
			t.Errorf("%s: %v", name, err)
		} else if got.PublicKeyID != tc.want {
			t.Errorf("%s: got %s, want %s", name, got.PublicKeyID, tc.want)
		}
	}
}
