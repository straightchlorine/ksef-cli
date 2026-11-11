package main

import (
	"bytes"
	"io"
	"os"
	"testing"
	"time"
)

func TestListInvoices(t *testing.T) {
	now := time.Date(2025, 8, 15, 0, 0, 0, 0, time.UTC)
	got, err := listInvoices(fakeKSeF(t, 200, 200), 2025, now)
	if err != nil {
		t.Fatal(err)
	}
	// both pages of the first quarter, in order
	if len(got) != 2 || got[0].KsefNumber != "KSEF-NUMBER-0" || got[1].KsefNumber != "KSEF-NUMBER-1" ||
		got[1].GrossAmount.String() != "100.5" || got[1].Buyer.Name != "Buyer" ||
		got[1].InvoicingMode != "Offline" {
		t.Errorf("got %+v", got)
	}
}

// Colours are for terminals only: a pipe or a file gets plain text.
func TestPaintPlainOffTerminal(t *testing.T) {
	_, w, _ := os.Pipe()
	defer w.Close()
	if got := paint(w, green, "[  OK  ]"); got != "[  OK  ]" {
		t.Errorf("got %q", got)
	}
}

func TestPull(t *testing.T) {
	cfg := fakeKSeF(t, 200, 200)
	conf := writeInvoice(t, "env=fake\nnip="+cfg.NIP+"\ncache_dir="+cfg.CacheDir+"\n")
	t.Setenv("KSEF_CONFIG", conf)
	t.Setenv("KSEF_TOKEN", cfg.Token)

	out := conf + ".pulled"
	if err := cmdPull([]string{"KSEF-NUMBER-1", out}); err != nil {
		t.Fatal(err)
	}
	if got, _ := os.ReadFile(out); string(got) != sampleInvoice {
		t.Errorf("pulled %q", got)
	}

	// no OUT_FILE: the invoice goes to stdout
	r, w, _ := os.Pipe()
	stdout := os.Stdout
	os.Stdout = w
	err := cmdPull([]string{"KSEF-NUMBER-1"})
	os.Stdout = stdout
	w.Close()
	if got, _ := io.ReadAll(r); err != nil || !bytes.Equal(got, []byte(sampleInvoice)) {
		t.Errorf("stdout %q, err %v", got, err)
	}
}
