package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

const sampleInvoice = `<?xml version="1.0" encoding="UTF-8"?>
<Faktura xmlns="http://crd.gov.pl/wzor/2025/06/25/13775/">
  <Naglowek>
    <KodFormularza kodSystemowy="FA (3)" wersjaSchemy="1-0E">FA</KodFormularza>
  </Naglowek>
  <Podmiot1><DaneIdentyfikacyjne><NIP>1234567890</NIP></DaneIdentyfikacyjne></Podmiot1>
  <Podmiot2><DaneIdentyfikacyjne><NIP>9999999999</NIP></DaneIdentyfikacyjne></Podmiot2>
  <Fa><P_1>2026-02-01</P_1></Fa>
</Faktura>
`

func writeInvoice(t *testing.T, content string) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "faktura.xml")
	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
		t.Fatal(err)
	}
	return path
}

func TestSend(t *testing.T) {
	cfg := fakeKSeF(t, 200, 200)
	cfg.NIP = "" // send falls back to the invoice's seller NIP
	path := writeInvoice(t, sampleInvoice)

	res, err := sendInvoice(cfg, path)
	if err != nil {
		t.Fatal(err)
	}
	if res.KsefNumber != "KSEF-NUMBER-1" {
		t.Errorf("KSeF number %q", res.KsefNumber)
	}
	if upo, _ := os.ReadFile(res.UPOPath); string(upo) != "<upo/>" ||
		res.UPOPath != strings.TrimSuffix(path, ".xml")+".upo.xml" {
		t.Errorf("UPO %q at %q", upo, res.UPOPath)
	}
	// seller NIP, P_1 as DD-MM-YYYY, then the unpadded base64url SHA-256
	hash := strings.NewReplacer("+", "-", "/", "_", "=", "").Replace(sha256B64([]byte(sampleInvoice)))
	if want := "/invoice/1234567890/01-02-2026/" + hash; res.VerifyURL != want {
		t.Errorf("verify URL %q, want %q", res.VerifyURL, want)
	}

	if _, err := sendInvoice(cfg, path); err == nil || !strings.Contains(err.Error(), "already sent") {
		t.Errorf("resending a file that has a UPO should be refused, got %v", err)
	}
}

func TestSendRejected(t *testing.T) {
	path := writeInvoice(t, sampleInvoice)
	_, err := sendInvoice(fakeKSeF(t, 200, 440), path)
	if err == nil || !strings.Contains(err.Error(), "ORIGINAL-KSEF-NUMBER") {
		t.Errorf("want a duplicate error naming the original invoice, got %v", err)
	}
	if _, statErr := os.Stat(strings.TrimSuffix(path, ".xml") + ".upo.xml"); statErr == nil {
		t.Error("a rejected invoice must not leave a UPO file behind")
	}
}

// Anything that is not an FA invoice is refused before any request is made.
func TestSendNotAnInvoice(t *testing.T) {
	for _, content := range []string{"not xml", "<Faktura/>", `<a><b>1</b></a>`} {
		if _, err := sendInvoice(&Config{Env: "test"}, writeInvoice(t, content)); err == nil {
			t.Errorf("%q should be refused", content)
		}
	}
}
