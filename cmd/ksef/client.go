package main

import (
	"bytes"
	"crypto/rsa"
	"crypto/x509"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"slices"
	"time"
)

var baseURLs = map[string]string{
	"test": "https://api-test.ksef.mf.gov.pl/v2",
	"demo": "https://api-demo.ksef.mf.gov.pl/v2",
	"prod": "https://api.ksef.mf.gov.pl/v2",
}

var httpClient = &http.Client{Timeout: 30 * time.Second}

// publicKeyCert is one entry of GET /security/public-key-certificates (unauthenticated).
// Certificate is base64 DER; Usage says what the key is for:
// "KsefTokenEncryption" for the auth flow,
// "SymmetricKeyEncryption" for the per-invoice AES key envelope.
type publicKeyCert struct {
	Certificate string    `json:"certificate"`
	PublicKeyID string    `json:"publicKeyId"`
	Usage       []string  `json:"usage"`
	ValidFrom   time.Time `json:"validFrom"`
	ValidTo     time.Time `json:"validTo"`
}

// fetchPublicKeyCerts returns the environment's certificate list, cached in
// cfg.CacheDir for 24h. A fetch failure falls back to a stale cache with a warning.
// (those certs are valid for a long time)
func fetchPublicKeyCerts(cfg *Config) ([]publicKeyCert, error) {
	cache := filepath.Join(cfg.CacheDir, "public-keys-"+cfg.Env+".json")
	if st, err := os.Stat(cache); err == nil && time.Since(st.ModTime()) < 24*time.Hour {
		if certs, err := readCerts(cache); err == nil {
			return certs, nil
		}
	}

	url := baseURLs[cfg.Env] + "/security/public-key-certificates"
	body, err := apiCall("GET", url, "", nil)
	if err != nil {
		if certs, cacheErr := readCerts(cache); cacheErr == nil {
			fmt.Fprintf(os.Stderr, "%s %v - using cached public keys\n",
				paint(os.Stderr, yellow, "[ WARN ]"), err)
			return certs, nil
		}
		return nil, err
	}
	var certs []publicKeyCert
	if err := json.Unmarshal(body, &certs); err != nil {
		return nil, fmt.Errorf("%s: %w", url, err)
	}
	if err := os.MkdirAll(cfg.CacheDir, 0o700); err == nil {
		_ = os.WriteFile(cache, body, 0o600)
	}
	return certs, nil
}

func readCerts(path string) ([]publicKeyCert, error) {
	b, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	var certs []publicKeyCert
	if err := json.Unmarshal(b, &certs); err != nil {
		return nil, err
	}
	return certs, nil
}

// pickCert selects which certificate to encrypt under for the given usage:
// valid right now, preferring the most recently activated - during a planned
// key rotation KSeF publishes the successor in advance, and encrypting under a
// not-yet-valid or expired key gets rejected.
//
// sections 3.2.1/4.1 at
// https://github.com/CIRFMF/ksef-api/blob/main/bezpieczenstwo/klucze-publiczne-do-szyfrowania.md
func pickCert(certs []publicKeyCert, usage string, now time.Time) (*publicKeyCert, error) {
	var best *publicKeyCert
	for i := range certs {
		c := &certs[i]
		if !slices.Contains(c.Usage, usage) {
			continue
		}
		if now.Before(c.ValidFrom) || now.After(c.ValidTo) {
			continue
		}
		if best == nil || c.ValidFrom.After(best.ValidFrom) {
			best = c
		}
	}
	if best == nil {
		return nil, fmt.Errorf("no currently-valid certificate with usage=%s", usage)
	}
	return best, nil
}

// publicKeyFor picks the environment's RSA public key for the given usage,
// with the publicKeyId that tells KSeF which key was used.
func publicKeyFor(cfg *Config, usage string) (*rsa.PublicKey, string, error) {
	certs, err := fetchPublicKeyCerts(cfg)
	if err != nil {
		return nil, "", err
	}
	c, err := pickCert(certs, usage, time.Now())
	if err != nil {
		return nil, "", err
	}
	der, err := base64.StdEncoding.DecodeString(c.Certificate)
	if err != nil {
		return nil, "", fmt.Errorf("certificate %s: %w", c.PublicKeyID, err)
	}
	cert, err := x509.ParseCertificate(der)
	if err != nil {
		return nil, "", fmt.Errorf("certificate %s: %w", c.PublicKeyID, err)
	}
	pub, ok := cert.PublicKey.(*rsa.PublicKey)
	if !ok {
		return nil, "", fmt.Errorf(
			"certificate %s: not an RSA key (%T)", c.PublicKeyID, cert.PublicKey,
		)
	}
	return pub, c.PublicKeyID, nil
}

// apiCall sends in as a JSON body (nil for none) and returns the response body.
// Any non-2xx status is an error carrying KSeF's own error body.
func apiCall(method, url, bearer string, in any) ([]byte, error) {
	var reqBody io.Reader
	if in != nil {
		b, err := json.Marshal(in)
		if err != nil {
			return nil, err
		}
		reqBody = bytes.NewReader(b)
	}
	req, err := http.NewRequest(method, url, reqBody)
	if err != nil {
		return nil, err
	}
	// NBP's API refuses Go's default User-Agent
	req.Header.Set("User-Agent", "ksef-cli/"+version)
	if in != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	if bearer != "" {
		req.Header.Set("Authorization", "Bearer "+bearer)
	}
	resp, err := httpClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("%s %s: %w", method, url, err)
	}
	if resp.StatusCode/100 != 2 {
		return nil, fmt.Errorf("%s %s: %s: %s", method, url, resp.Status, body)
	}
	return body, nil
}

// apiJSON is apiCall with the response decoded into out.
func apiJSON(method, url, bearer string, in, out any) error {
	body, err := apiCall(method, url, bearer, in)
	if err != nil {
		return err
	}
	if err := json.Unmarshal(body, out); err != nil {
		return fmt.Errorf("%s %s: %w", method, url, err)
	}
	return nil
}
