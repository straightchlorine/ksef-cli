package main

import (
	"bytes"
	"encoding/json"
	"io"
	"os"
	"regexp"
	"testing"
)

// captured returns what print wrote, as a pipe sees it: no colour codes.
func captured(t *testing.T, print func(w *os.File)) string {
	t.Helper()
	r, w, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	print(w)
	w.Close()
	out, _ := io.ReadAll(r)
	return string(out)
}

func TestPrintList(t *testing.T) {
	inv := func(date, number, gross, buyer, mode string) invoiceMeta {
		m := invoiceMeta{IssueDate: date, InvoiceNumber: number, Currency: "EUR",
			GrossAmount: json.Number(gross), InvoicingMode: mode, KsefNumber: "K-" + number}
		m.Buyer.Name = buyer
		return m
	}
	got := captured(t, func(w *os.File) {
		printList(w, []invoiceMeta{
			inv("2026-04-02", "FV/1", "3333", "Müller GmbH", "Online"),
			inv("2026-11-01", "FV/10", "120.5",
				"A very long buyer name that runs past the limit", "Offline"),
		})
	})
	want := "" +
		"DATE        NUMBER        GROSS  BUYER                             MODE     KSEF NUMBER\n" +
		"2026-04-02  FV/1    3333.00 EUR  Müller GmbH                       Online   K-FV/1\n" +
		"2026-11-01  FV/10    120.50 EUR  A very long buyer name that run…  Offline  K-FV/10\n"
	if got != want {
		t.Errorf("got\n%s\nwant\n%s", got, want)
	}
}

func TestPrintSummary(t *testing.T) {
	head, err := parseInvoiceHead([]byte(sampleInvoice))
	if err != nil {
		t.Fatal(err)
	}
	head.Number, head.Total, head.Currency = "FV/1", "10.00", "EUR"
	head.Buyer.KodUE, head.Buyer.NrVatUE, head.Buyer.Name = "DE", "123", "Kupiec"
	head.Buyer.NIP = ""
	head.SaleFrom, head.SaleTo = "2026-01-01", "2026-01-31"
	head.Lines = []invoiceLine{{Name: "Usługa", Quantity: "2", Price: "5.00", Rate: "4.2"},
		{Name: "Dojazd", Quantity: "1", Unit: "szt."}, {}, {}, {}}
	got := captured(t, func(w *os.File) { printSummary(w, head) })
	want := "" +
		"  invoice: FV/1, issued 2026-02-01, sale 2026-01-01 to 2026-01-31\n" +
		"  seller:  NIP 1234567890\n" +
		"  buyer:   Kupiec, VAT DE123\n" +
		"  item:    Usługa, 2 x 5.00\n" +
		"  item:    Dojazd, 1 szt.\n" +
		"  item:    ... and 2 more\n" +
		"  total:   10.00 EUR at 4.2\n"
	if got != want {
		t.Errorf("got\n%s\nwant\n%s", got, want)
	}
}

// Colouring only adds codes: with them stripped, the XML is what it was.
func TestColourXML(t *testing.T) {
	in := sampleInvoice + `<a x="1 > 0" y='2'/><!-- c --><b>t &amp; u</b>`
	got := colourXML([]byte(in))
	if plain := regexp.MustCompile(`\x1b\[[0-9;]*m`).ReplaceAll(got, nil); string(plain) != in {
		t.Errorf("colouring changed the text:\n%s", plain)
	}
	want := "\x1b[2m<\x1b[0m\x1b[36mKodFormularza\x1b[0m " +
		"\x1b[33mkodSystemowy\x1b[0m\x1b[2m=\x1b[0m\x1b[32m\"FA (3)\"\x1b[0m"
	if !bytes.Contains(got, []byte(want)) {
		t.Errorf("tag name, attribute and value are not coloured as expected:\n%q", got)
	}
}

// Bodies as the real API returned them.
func TestAPIError(t *testing.T) {
	for body, want := range map[string]string{
		`{"exception":{"exceptionDetailList":[{"exceptionCode":21164,"exceptionDescription":` +
			`"Faktura nie istnieje.","details":["Nie została znaleziona."]}],"serviceCode":"x"}}`: "" +
			"21164 Faktura nie istnieje. Nie została znaleziona.",
		`{"status":{"code":429,"description":"Too Many Requests","details":["Spróbuj za 3 min."]}}`: "" +
			"Spróbuj za 3 min.",
		`{"title":"Forbidden","status":403,"detail":"Brak uprawnień."}`: "Forbidden: Brak uprawnień.",
		"<html>502</html>\n": "<html>502</html>",
	} {
		if got := apiError([]byte(body)); got != want {
			t.Errorf("apiError(%s) = %q, want %q", body, got, want)
		}
	}
}
