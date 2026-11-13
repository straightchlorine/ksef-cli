package main

import (
	"bytes"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestListInvoices(t *testing.T) {
	cfg := fakeKSeF(t, 200, 200)
	now := time.Date(2025, 8, 15, 0, 0, 0, 0, time.UTC)
	from, to, err := parsePeriod("2025", now)
	if err != nil {
		t.Fatal(err)
	}
	got, err := listInvoices(cfg, false, from, to, now)
	if err != nil {
		t.Fatal(err)
	}
	// three quarters have begun by August: two pages in the first, one in each other
	if fakeQueries != 4 {
		t.Errorf("%d queries for a year, want 4", fakeQueries)
	}
	// both pages of the first quarter, in order
	if len(got) != 2 || got[0].KsefNumber != "KSEF-NUMBER-0" || got[1].KsefNumber != "KSEF-NUMBER-1" ||
		got[1].GrossAmount.String() != "100.5" || got[1].Buyer.Name != "Buyer" ||
		got[1].InvoicingMode != "Offline" {
		t.Errorf("got %+v", got)
	}
}

// A month is one query range, and --received asks for the buyer's side.
func TestListReceivedMonth(t *testing.T) {
	cfg := fakeKSeF(t, 200, 200)
	now := time.Date(2025, 8, 15, 0, 0, 0, 0, time.UTC)
	from, to, err := parsePeriod("2025-01", now)
	if err != nil {
		t.Fatal(err)
	}
	got, err := listInvoices(cfg, true, from, to, now)
	if err != nil {
		t.Fatal(err)
	}
	if fakeQueries != 2 || len(got) != 2 || got[0].InvoiceNumber != "IN/0" ||
		got[0].Seller.Name != "Seller" {
		t.Errorf("%d queries, got %+v", fakeQueries, got)
	}
}

func TestParsePeriod(t *testing.T) {
	now := time.Date(2026, 11, 13, 9, 0, 0, 0, time.UTC)
	for in, want := range map[string][2]string{
		"":        {"2026-01-01", "2027-01-01"},
		"2025":    {"2025-01-01", "2026-01-01"},
		"2026-11": {"2026-11-01", "2026-12-01"},
		"2025-12": {"2025-12-01", "2026-01-01"},
	} {
		from, to, err := parsePeriod(in, now)
		got := [2]string{from.Format(time.DateOnly), to.Format(time.DateOnly)}
		if err != nil || got != want {
			t.Errorf("parsePeriod(%q) = %v, %v; want %v", in, got, err, want)
		}
	}
	for _, bad := range []string{"1999", "2027", "2026-12", "2026-1", "2026-13", "11-2026", "last"} {
		if _, _, err := parsePeriod(bad, now); err == nil {
			t.Errorf("parsePeriod(%q) should fail", bad)
		}
	}
}

// useFake points the commands' own config loading at the fake server.
func useFake(t *testing.T, cfg *Config) {
	t.Helper()
	conf := writeInvoice(t, "env=fake\nnip="+cfg.NIP+"\ncache_dir="+cfg.CacheDir+"\n")
	t.Setenv("KSEF_CONFIG", conf)
	t.Setenv("KSEF_TOKEN", cfg.Token)
}

func TestStatus(t *testing.T) {
	useFake(t, fakeKSeF(t, 200, 200))
	upo := filepath.Join(t.TempDir(), "faktura.upo.xml")
	if err := cmdStatus([]string{"20260301-SO-0000000001-0000000001-AA", upo}); err != nil {
		t.Fatal(err)
	}
	if got, _ := os.ReadFile(upo); string(got) != "<upo/>" {
		t.Errorf("UPO %q", got)
	}
	if err := cmdStatus([]string{"20260301-SO-0000000001-0000000001-AA", upo}); err == nil {
		t.Error("an existing UPO file must not be overwritten")
	}
	if err := cmdStatus([]string{"20260301-SO-0000000001-0000000001-AA"}); err != nil {
		t.Errorf("without UPO_FILE: %v", err)
	}
	fakeSessionEmpty = true
	err := cmdStatus([]string{"20260301-SO-0000000001-0000000001-AA"})
	if err == nil || !strings.Contains(err.Error(), "no invoice") {
		t.Errorf("an empty session should say so, got %v", err)
	}
}

// An invoice that is not accepted is an error, and leaves no UPO behind.
func TestStatusNotAccepted(t *testing.T) {
	for code, want := range map[int]string{150: "still processing", 440: "rejected"} {
		useFake(t, fakeKSeF(t, 200, code))
		upo := filepath.Join(t.TempDir(), "faktura.upo.xml")
		err := cmdStatus([]string{"20260301-SO-0000000001-0000000001-AA", upo})
		if err == nil || !strings.Contains(err.Error(), want) {
			t.Errorf("status %d: want %q, got %v", code, want, err)
		}
		if _, statErr := os.Stat(upo); statErr == nil {
			t.Errorf("status %d: a UPO file was written", code)
		}
	}
}

func TestPull(t *testing.T) {
	cfg := fakeKSeF(t, 200, 200)
	conf := writeInvoice(t, "env=fake\nnip="+cfg.NIP+"\ncache_dir="+cfg.CacheDir+"\n")
	t.Setenv("KSEF_CONFIG", conf)
	t.Setenv("KSEF_TOKEN", cfg.Token)

	out := conf + ".pulled"
	if err := cmdPull([]string{"1234563218-20260301-0123456789AB-CD", out}); err != nil {
		t.Fatal(err)
	}
	if got, _ := os.ReadFile(out); string(got) != sampleInvoice {
		t.Errorf("pulled %q", got)
	}

	// no OUT_FILE: the invoice goes to stdout
	r, w, _ := os.Pipe()
	stdout := os.Stdout
	os.Stdout = w
	err := cmdPull([]string{"1234563218-20260301-0123456789AB-CD"})
	os.Stdout = stdout
	w.Close()
	if got, _ := io.ReadAll(r); err != nil || !bytes.Equal(got, []byte(sampleInvoice)) {
		t.Errorf("stdout %q, err %v", got, err)
	}
}
