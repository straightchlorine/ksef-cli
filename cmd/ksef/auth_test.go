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
	"os"
	"strings"
	"testing"
	"time"
)

// Faults the fake server injects: the invoice request is dropped without an
// answer, the status call fails this many times, the first logins get no
// answer, the session holds no invoice. fakeQueries counts metadata queries.
var (
	fakeDropsSend      bool
	fakeUPOFails       bool
	fakeSessionEmpty   bool
	fakeStatusFailures int
	fakeLoginDrops     int
	fakeQueries        int
	// the invoice a send must carry; empty for any
	fakeInvoice = sampleInvoice
)

// hangUp takes the request and cuts the line without answering.
func hangUp(w http.ResponseWriter) {
	conn, _, _ := w.(http.Hijacker).Hijack()
	conn.Close()
}

// fakeKSeF serves the auth and send endpoints the way the real API does and
// fails the test when a request is malformed. authCode and invoiceCode are the
// statuses the login and the sent invoice end with.
func fakeKSeF(t *testing.T, authCode, invoiceCode int) *Config {
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
	mux := http.NewServeMux()
	// on registers a handler that insists on the given bearer token ("" for none).
	on := func(pattern, bearer string, h http.HandlerFunc) {
		mux.HandleFunc(pattern, func(w http.ResponseWriter, r *http.Request) {
			if got := r.Header.Get("Authorization"); bearer != "" && got != "Bearer "+bearer {
				t.Errorf("%s: bad Authorization %q", r.URL.Path, got)
				w.WriteHeader(http.StatusUnauthorized)
				return
			}
			h(w, r)
		})
	}

	on("GET /security/public-key-certificates", "", func(w http.ResponseWriter, _ *http.Request) {
		reply(w, 200, []publicKeyCert{{
			Certificate: base64.StdEncoding.EncodeToString(der),
			PublicKeyID: "key-1",
			Usage:       []string{"KsefTokenEncryption", "SymmetricKeyEncryption"},
			ValidFrom:   tmpl.NotBefore,
			ValidTo:     tmpl.NotAfter,
		}})
	})
	on("POST /auth/challenge", "", func(w http.ResponseWriter, _ *http.Request) {
		if fakeLoginDrops > 0 {
			fakeLoginDrops--
			hangUp(w)
			return
		}
		reply(w, 200, map[string]any{"challenge": "chal-1", "timestampMs": 1700000000123})
	})
	on("POST /auth/ksef-token", "", func(w http.ResponseWriter, r *http.Request) {
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
	authPolls := 0
	on("GET /auth/ref-1", "auth-token", func(w http.ResponseWriter, _ *http.Request) {
		code := authCode
		if authPolls++; authPolls == 1 {
			code = 100
		}
		reply(w, 200, map[string]any{"status": statusInfo{Code: code, Description: "desc"}})
	})
	on("POST /auth/token/redeem", "auth-token", func(w http.ResponseWriter, _ *http.Request) {
		reply(w, 200, map[string]any{"accessToken": map[string]string{"token": "access-token"}})
	})

	var aesKey, aesIV []byte
	on("POST /sessions/online", "access-token", func(w http.ResponseWriter, r *http.Request) {
		var req struct {
			FormCode   struct{ SystemCode, SchemaVersion, Value string }
			Encryption struct{ EncryptedSymmetricKey, InitializationVector, PublicKeyID string }
		}
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			t.Errorf("open session body: %v", err)
		}
		if req.FormCode.SystemCode != "FA (3)" || req.FormCode.SchemaVersion != "1-0E" ||
			req.FormCode.Value != "FA" || req.Encryption.PublicKeyID != "key-1" {
			t.Errorf("open session request: %+v", req)
		}
		wrapped, _ := base64.StdEncoding.DecodeString(req.Encryption.EncryptedSymmetricKey)
		k, err := rsa.DecryptOAEP(sha256.New(), nil, key, wrapped, nil)
		if err != nil {
			t.Errorf("encryptedSymmetricKey: %v", err)
		}
		aesKey = k
		aesIV, _ = base64.StdEncoding.DecodeString(req.Encryption.InitializationVector)
		reply(w, 201, map[string]string{"referenceNumber": "20260301-SO-0000000001-0000000001-AA"})
	})
	invoices := "/sessions/online/20260301-SO-0000000001-0000000001-AA/invoices"
	on("POST "+invoices, "access-token", func(w http.ResponseWriter, r *http.Request) {
		if fakeDropsSend {
			hangUp(w)
			return
		}
		var req struct {
			InvoiceHash, EncryptedInvoiceHash, EncryptedInvoiceContent string
			InvoiceSize, EncryptedInvoiceSize                          int
		}
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			t.Errorf("send invoice body: %v", err)
		}
		ct, _ := base64.StdEncoding.DecodeString(req.EncryptedInvoiceContent)
		plain, err := aesDecrypt(aesKey, aesIV, ct)
		if err != nil || fakeInvoice != "" && string(plain) != fakeInvoice ||
			sha256B64(plain) != req.InvoiceHash || len(plain) != req.InvoiceSize ||
			sha256B64(ct) != req.EncryptedInvoiceHash || len(ct) != req.EncryptedInvoiceSize {
			t.Errorf("invoice content, hashes or sizes are off (decrypt err %v)", err)
		}
		reply(w, 202, map[string]string{"referenceNumber": "inv-1"})
	})
	on("POST /sessions/online/20260301-SO-0000000001-0000000001-AA/close", "access-token",
		func(w http.ResponseWriter, _ *http.Request) { w.WriteHeader(http.StatusNoContent) })
	invoicePolls := 0
	sentInvoice := "GET /sessions/20260301-SO-0000000001-0000000001-AA/invoices/inv-1"
	on(sentInvoice, "access-token", func(w http.ResponseWriter, _ *http.Request) {
		if fakeStatusFailures > 0 {
			fakeStatusFailures--
			w.WriteHeader(http.StatusBadGateway)
			return
		}
		st := statusInfo{Code: invoiceCode, Description: "desc"}
		if invoicePolls++; invoicePolls == 1 {
			st.Code = 100
		}
		if st.Code == 440 {
			st.Extensions = map[string]string{"originalKsefNumber": "ORIGINAL-KSEF-NUMBER"}
		}
		reply(w, 200, map[string]any{"ksefNumber": "KSEF-NUMBER-1", "status": st})
	})
	on(sentInvoice+"/upo", "access-token", func(w http.ResponseWriter, _ *http.Request) {
		if fakeUPOFails {
			w.WriteHeader(http.StatusNotFound)
			return
		}
		w.Write([]byte("<upo/>"))
	})

	// what `ksef status` reads: the session's invoices, or the session itself
	sessionInvoices := "GET /sessions/20260301-SO-0000000001-0000000001-AA/invoices"
	on(sessionInvoices, "access-token", func(w http.ResponseWriter, _ *http.Request) {
		invoices := []invoiceState{}
		if !fakeSessionEmpty {
			invoices = append(invoices, invoiceState{
				ReferenceNumber: "inv-1", InvoiceNumber: "FV/1", KsefNumber: "KSEF-NUMBER-1",
				Status: statusInfo{Code: invoiceCode, Description: "desc"}})
		}
		reply(w, 200, map[string]any{"invoices": invoices})
	})
	sessionItself := "GET /sessions/20260301-SO-0000000001-0000000001-AA"
	on(sessionItself, "access-token", func(w http.ResponseWriter, _ *http.Request) {
		reply(w, 200, map[string]any{"status": statusInfo{Code: 440, Description: "Sesja anulowana"}})
	})

	// Two pages in the first quarter, nothing in the rest.
	on("POST /invoices/query/metadata", "access-token", func(w http.ResponseWriter, r *http.Request) {
		var req struct {
			SubjectType string
			DateRange   struct{ DateType, From, To string }
		}
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			t.Errorf("metadata query body: %v", err)
		}
		from, _ := time.Parse(time.RFC3339, req.DateRange.From)
		to, err := time.Parse(time.RFC3339, req.DateRange.To)
		fakeQueries++
		received := req.SubjectType == "Subject2"
		if !received && req.SubjectType != "Subject1" ||
			req.DateRange.DateType != "Issue" || err != nil ||
			!to.After(from) || to.After(from.AddDate(0, 3, 0)) {
			t.Errorf("metadata query: %+v", req)
		}
		page := r.URL.Query().Get("pageOffset")
		resp := map[string]any{"hasMore": false, "isTruncated": false, "invoices": []any{}}
		if from.Month() == time.January {
			resp["hasMore"] = page == "0"
			resp["invoices"] = []map[string]any{{
				"ksefNumber": "KSEF-NUMBER-" + page, "invoiceNumber": "FV/" + page,
				"issueDate": "2025-01-1" + page, "grossAmount": 100.50, "currency": "PLN",
				"buyer": map[string]string{"name": "Buyer"}, "invoicingMode": "Offline",
				"seller": map[string]string{"name": "Seller"},
			}}
			if received {
				resp["invoices"].([]map[string]any)[0]["invoiceNumber"] = "IN/" + page
			}
		}
		reply(w, 200, resp)
	})
	stored := "GET /invoices/ksef/1234563218-20260301-0123456789AB-CD"
	on(stored, "access-token", func(w http.ResponseWriter, _ *http.Request) {
		w.Write([]byte(sampleInvoice))
	})

	srv := httptest.NewServer(mux)
	baseURLs["fake"] = srv.URL
	oldInterval := pollInterval
	pollInterval = 0
	t.Cleanup(func() {
		srv.Close()
		delete(baseURLs, "fake")
		pollInterval = oldInterval
		fakeDropsSend, fakeUPOFails, fakeSessionEmpty = false, false, false
		fakeStatusFailures, fakeLoginDrops, fakeQueries = 0, 0, 0
		fakeInvoice, stdin = sampleInvoice, os.Stdin
	})
	return &Config{Env: "fake", NIP: "1234567890", Token: "secret-token", CacheDir: t.TempDir()}
}

func TestAuthenticate(t *testing.T) {
	got, err := authenticate(fakeKSeF(t, 200, 200))
	if err != nil {
		t.Fatal(err)
	}
	if got != "access-token" {
		t.Errorf("got %q", got)
	}
}

func TestAuthenticateRejected(t *testing.T) {
	_, err := authenticate(fakeKSeF(t, 450, 200))
	if err == nil || !strings.Contains(err.Error(), "450") ||
		!strings.Contains(err.Error(), "env=fake") {
		t.Errorf("want a login-rejected error carrying the status code, got %v", err)
	}
}

// A login that gets no answer is tried again, three times in all.
func TestAuthenticateRetries(t *testing.T) {
	cfg := fakeKSeF(t, 200, 200)
	fakeLoginDrops = 2
	if got, err := authenticate(cfg); err != nil || got != "access-token" {
		t.Errorf("two unanswered logins should be ridden out, got %q, %v", got, err)
	}
	fakeLoginDrops = 3
	if _, err := authenticate(cfg); err == nil {
		t.Error("three unanswered logins should be an error")
	}
	if fakeLoginDrops != 0 {
		t.Errorf("%d unanswered logins were never tried", fakeLoginDrops)
	}
}

func TestAuthenticateNeedsCredentials(t *testing.T) {
	if _, err := authenticate(&Config{Env: "test"}); err == nil {
		t.Error("missing nip/token should be an error before any request is made")
	}
}
