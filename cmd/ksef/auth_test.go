package main

import (
	"crypto/rand"
	"crypto/rsa"
	"crypto/sha256"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/base64"
	"encoding/json"
	"math/big"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

// fakeKSeF serves the auth endpoints the way the real API does and fails the
// test when a request is malformed. finalCode is the status the login ends with.
func fakeKSeF(t *testing.T, finalCode int) *Config {
	t.Helper()
	key, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatal(err)
	}
	now := time.Now()
	tmpl := &x509.Certificate{
		SerialNumber: big.NewInt(1),
		Subject:      pkix.Name{CommonName: "fake ksef"},
		NotBefore:    now.Add(-time.Hour),
		NotAfter:     now.Add(time.Hour),
	}
	der, err := x509.CreateCertificate(rand.Reader, tmpl, tmpl, &key.PublicKey, key)
	if err != nil {
		t.Fatal(err)
	}
	reply := func(w http.ResponseWriter, status int, v any) {
		w.WriteHeader(status)
		json.NewEncoder(w).Encode(v)
	}
	wantBearer := func(w http.ResponseWriter, r *http.Request) bool {
		if r.Header.Get("Authorization") != "Bearer auth-token" {
			t.Errorf("%s: bad Authorization %q", r.URL.Path, r.Header.Get("Authorization"))
			w.WriteHeader(http.StatusUnauthorized)
			return false
		}
		return true
	}

	polls := 0
	mux := http.NewServeMux()
	certsPath := "GET /security/public-key-certificates"
	mux.HandleFunc(certsPath, func(w http.ResponseWriter, _ *http.Request) {
		reply(w, 200, []publicKeyCert{{
			Certificate: base64.StdEncoding.EncodeToString(der),
			PublicKeyID: "key-1",
			Usage:       []string{"KsefTokenEncryption"},
			ValidFrom:   tmpl.NotBefore,
			ValidTo:     tmpl.NotAfter,
		}})
	})
	mux.HandleFunc("POST /auth/challenge", func(w http.ResponseWriter, _ *http.Request) {
		reply(w, 200, map[string]any{"challenge": "chal-1", "timestampMs": 1700000000123})
	})
	mux.HandleFunc("POST /auth/ksef-token", func(w http.ResponseWriter, r *http.Request) {
		var req struct {
			Challenge         string
			ContextIdentifier struct{ Type, Value string }
			EncryptedToken    string
			PublicKeyID       string `json:"publicKeyId"`
		}
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			t.Errorf("ksef-token body: %v", err)
		}
		if req.Challenge != "chal-1" || req.ContextIdentifier.Type != "Nip" ||
			req.ContextIdentifier.Value != "1234567890" || req.PublicKeyID != "key-1" {
			t.Errorf("ksef-token request: %+v", req)
		}
		ct, _ := base64.StdEncoding.DecodeString(req.EncryptedToken)
		plain, err := rsa.DecryptOAEP(sha256.New(), nil, key, ct, nil)
		if err != nil || string(plain) != "secret-token|1700000000123" {
			t.Errorf("encryptedToken decrypts to %q, err %v", plain, err)
		}
		reply(w, 202, map[string]any{
			"referenceNumber":     "ref-1",
			"authenticationToken": map[string]string{"token": "auth-token"},
		})
	})
	mux.HandleFunc("GET /auth/ref-1", func(w http.ResponseWriter, r *http.Request) {
		if !wantBearer(w, r) {
			return
		}
		code := finalCode
		if polls++; polls == 1 {
			code = 100
		}
		reply(w, 200, map[string]any{"status": statusInfo{Code: code, Description: "desc"}})
	})
	mux.HandleFunc("POST /auth/token/redeem", func(w http.ResponseWriter, r *http.Request) {
		if !wantBearer(w, r) {
			return
		}
		reply(w, 200, map[string]any{"accessToken": map[string]string{"token": "access-token"}})
	})

	srv := httptest.NewServer(mux)
	baseURLs["fake"] = srv.URL
	oldInterval := pollInterval
	pollInterval = 0
	t.Cleanup(func() {
		srv.Close()
		delete(baseURLs, "fake")
		pollInterval = oldInterval
	})
	return &Config{Env: "fake", NIP: "1234567890", Token: "secret-token", CacheDir: t.TempDir()}
}

func TestAuthenticate(t *testing.T) {
	got, err := authenticate(fakeKSeF(t, 200))
	if err != nil {
		t.Fatal(err)
	}
	if got != "access-token" {
		t.Errorf("got %q", got)
	}
}

func TestAuthenticateRejected(t *testing.T) {
	_, err := authenticate(fakeKSeF(t, 450))
	if err == nil || !strings.Contains(err.Error(), "450") {
		t.Errorf("want a login-rejected error carrying the status code, got %v", err)
	}
}

func TestAuthenticateNeedsCredentials(t *testing.T) {
	if _, err := authenticate(&Config{Env: "test"}); err == nil {
		t.Error("missing nip/token should be an error before any request is made")
	}
}
