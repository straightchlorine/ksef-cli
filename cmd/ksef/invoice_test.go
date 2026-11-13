package main

import (
	"math"
	"strings"
	"testing"
)

// An invoice built by hand, with no Invoify involved, renders to FA(3).
func TestInvoiceRender(t *testing.T) {
	inv := &Invoice{
		Number: "FV/1", Currency: "eur", Issued: "2026-03-01", Rate: "4.25",
		PlaceOfIssue: "Kraków", OrderNumber: "PO-7",
		Seller: Party{Name: "Jan", Addr1: "Prosta 1", Addr2: "00-001 Warszawa", NIP: "123-456-32-18"},
		Buyer: Party{Name: "Kupiec & Co", Addr1: "Str. 1", Addr2: "Berlin", VAT: "de 123",
			Country: "Niemcy"},
		Lines: []Line{{Name: "Usługa", Unit: "godz.", Quantity: 1.5, UnitPrice: 100},
			// 0.1+0.2 is 0.30000000000000004 as a float; a price finer than a grosz
			{Name: "Drobne", Quantity: 0.1 + 0.2, UnitPrice: 33.3333}},
		Account: "PL61109010140000071219812874", SWIFT: "WBKPPLPP",
		Extras: []Extra{{Key: "Uwagi", Value: xmlText(strings.Repeat("ż", 300))}},
	}
	if err := inv.normalise(); err != nil {
		t.Fatal(err)
	}
	if err := inv.validate(); err != nil {
		t.Fatal(err)
	}
	out, err := inv.render(testNow)
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{
		"<NIP>1234563218</NIP>",
		"<KodUE>DE</KodUE>\n      <NrVatUE>123</NrVatUE>\n      <Nazwa>Kupiec &amp; Co</Nazwa>",
		"<KodKraju>DE</KodKraju>\n      <AdresL1>Str. 1",
		"<KodWaluty>EUR</KodWaluty>\n    <P_1>2026-03-01</P_1>\n    <P_1M>Kraków</P_1M>",
		"<P_15>160.00</P_15>",
		"<P_8B>0.3</P_8B>\n      <P_9B>33.3333</P_9B>\n      <P_11A>10.00</P_11A>",
		// a value over 256 characters is split into two entries
		"<Wartosc>" + strings.Repeat("ż", 256) + "</Wartosc>",
		"<Wartosc>" + strings.Repeat("ż", 44) + "</Wartosc>",
		"<P_8A>godz.</P_8A>\n      <P_8B>1.5</P_8B>\n      <P_9B>100.00</P_9B>",
		"<KursWaluty>4.25</KursWaluty>",
		"<NrRB>PL61109010140000071219812874</NrRB>\n        <SWIFT>WBKPPLPP</SWIFT>",
		"<NrZamowienia>PO-7</NrZamowienia>",
		"<P_19A>art. 113 ust. 1 ",
	} {
		if !strings.Contains(string(out), want) {
			t.Errorf("missing %s\n%s", want, out)
		}
	}
	if _, err := parseInvoiceHead(out); err != nil {
		t.Errorf("the result does not read back as an FA invoice: %v", err)
	}
}

// The three numbers on a line agree: printed quantity x printed price, as
// exact decimals rounded half up, is the printed sum.
func TestLineSums(t *testing.T) {
	for _, tc := range []struct {
		qty, price            float64
		wantQty, wantP, wantS string
	}{
		{1, 1.005, "1", "1.005", "1.01"},
		{3, 0.335, "3", "0.335", "1.01"},
		{0.1234567, 1000000, "0.123457", "1000000.00", "123457.00"},
		{0.1 + 0.2, 33.3333, "0.3", "33.3333", "10.00"},
		{1.5, 100, "1.5", "100.00", "150.00"},
		{7, 0.01, "7", "0.01", "0.07"},
		{123456789, 12345.67, "123456789", "12345.67", "1524156776253.63"},
	} {
		l := Line{Quantity: tc.qty, UnitPrice: tc.price}
		if l.Qty() != tc.wantQty || l.Price() != tc.wantP || l.Sum() != tc.wantS {
			t.Errorf("%v x %v: got %s x %s = %s, want %s x %s = %s", tc.qty, tc.price,
				l.Qty(), l.Price(), l.Sum(), tc.wantQty, tc.wantP, tc.wantS)
		}
	}
	inv := Invoice{Lines: []Line{{Quantity: 1, UnitPrice: 0.1}, {Quantity: 1, UnitPrice: 0.2}}}
	if inv.Net() != "0.30" || inv.Gross() != "0.30" {
		t.Errorf("net %s, gross %s", inv.Net(), inv.Gross())
	}
}

// An invoice with VAT: net prices, VAT on their sum, gross total. The numbers
// are those of the official FA(3) sample invoice.
func TestInvoiceVAT(t *testing.T) {
	inv := &Invoice{
		Number: "FV/1", Currency: "PLN", Issued: "2026-03-01", VATRate: 23,
		Seller: Party{Name: "S", Addr1: "A 1", NIP: "1234563218"},
		Buyer:  Party{Name: "B", Addr1: "B 2", NIP: "9999999999"},
		Lines: []Line{{Name: "Energia", Unit: "kWh", Quantity: 20, UnitPrice: 0.90},
			{Name: "Dystrybucja", Quantity: 1, UnitPrice: 25.60}},
		DeclaredTotal: 53.628, // unrounded, as Invoify may carry it
	}
	if err := inv.normalise(); err != nil {
		t.Fatal(err)
	}
	if err := inv.validate(); err != nil {
		t.Fatal(err)
	}
	out, err := inv.render(testNow)
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{
		"<P_13_1>43.60</P_13_1>\n    <P_14_1>10.03</P_14_1>\n    <P_15>53.63</P_15>",
		"<Zwolnienie>\n        <P_19N>1</P_19N>\n      </Zwolnienie>",
		"<P_8B>20</P_8B>\n      <P_9A>0.90</P_9A>\n      <P_11>18.00</P_11>\n      <P_12>23</P_12>",
	} {
		if !strings.Contains(string(out), want) {
			t.Errorf("missing %s\n%s", want, out)
		}
	}
	for _, unwanted := range []string{"P_13_7", "P_19A", "P_9B", "P_11A", ">zw<"} {
		if strings.Contains(string(out), unwanted) {
			t.Errorf("an invoice with VAT should not have %s", unwanted)
		}
	}
}

// Each rate has its own pair of total fields; VAT is rounded half up.
func TestVATTotals(t *testing.T) {
	for _, tc := range []struct {
		rate       int
		net        float64
		bucket     int
		vat, gross string
	}{
		{23, 1200, 1, "276.00", "1476.00"},
		{8, 100, 2, "8.00", "108.00"},
		{5, 19.99, 3, "1.00", "20.99"},
		{23, 0.50, 1, "0.12", "0.62"}, // 0.115 rounds up
		{23, 0.02, 1, "0.00", "0.02"}, // 0.0046 rounds down
		{0, 100, 0, "0.00", "100.00"},
	} {
		inv := Invoice{VATRate: tc.rate, Lines: []Line{{Quantity: 1, UnitPrice: tc.net}}}
		if inv.VATBucket() != tc.bucket || inv.VAT() != tc.vat || inv.Gross() != tc.gross {
			t.Errorf("%d%% of %v: bucket %d, VAT %s, gross %s", tc.rate, tc.net,
				inv.VATBucket(), inv.VAT(), inv.Gross())
		}
	}
}

func TestValidNIP(t *testing.T) {
	for nip, want := range map[string]bool{
		"1234563218": true, "1111111111": true, "1234567890": false, "1234563219": false,
		"123456321": false, "12345632180": false, "123456321X": false, "": false,
	} {
		if validNIP(nip) != want {
			t.Errorf("validNIP(%q) = %v", nip, !want)
		}
	}
}

// Tax ids land in the right field whatever they were typed as.
func TestNormaliseIDs(t *testing.T) {
	for name, tc := range map[string]struct {
		in                       Party
		nip, vat, other, country string
	}{
		"NIP with separators": {Party{NIP: "123-456 32-18"}, "1234563218", "", "", "PL"},
		"NIP with PL prefix":  {Party{NIP: "pl1234563218"}, "1234563218", "", "", "PL"},
		"PL VAT number":       {Party{VAT: "PL 1234563218"}, "1234563218", "", "", "PL"},
		"EU VAT number":       {Party{VAT: "de 123456789"}, "", "DE123456789", "", "DE"},
		"Greek VAT number": {
			Party{VAT: "EL123456789", Country: "Greece"}, "", "EL123456789", "", "GR"},
		"non-EU tax number": {Party{VAT: "GB123456789"}, "", "", "GB123456789", "GB"},
		"non-EU, country set": {
			Party{VAT: "CHE123456", Country: "Szwajcaria"}, "", "", "CHE123456", "CH"},
	} {
		inv := Invoice{Issued: "2026-03-01", Buyer: tc.in}
		if err := inv.normalise(); err != nil {
			t.Errorf("%s: %v", name, err)
			continue
		}
		b := inv.Buyer
		if b.NIP != tc.nip || b.VAT != tc.vat || string(b.OtherID) != tc.other ||
			b.Country != tc.country {
			t.Errorf("%s: got NIP %q VAT %q other %q country %q",
				name, b.NIP, b.VAT, b.OtherID, b.Country)
		}
	}
}

// Invoify stamps a date as local midnight in UTC; a plain date stays as it is.
func TestPolishDate(t *testing.T) {
	for in, want := range map[string]string{
		"2026-03-01":               "2026-03-01",
		"2026-02-28T23:00:00.000Z": "2026-03-01", // Warsaw midnight, winter
		"2026-06-30T22:00:00.000Z": "2026-07-01", // Warsaw midnight, summer
		"2026-11-01T00:00:00.000Z": "2026-11-01", // UTC midnight
		"2026-11-01T14:30:00.000Z": "2026-11-01",
		"November 1, 2026":         "2026-11-01",
	} {
		if got, err := polishDate(in); err != nil || got != want {
			t.Errorf("polishDate(%q) = %q, %v; want %q", in, got, err, want)
		}
	}
	for _, bad := range []string{"", "01.03.2026", "2026-13-01", "2026-02-30"} {
		if _, err := polishDate(bad); err == nil {
			t.Errorf("polishDate(%q) should fail", bad)
		}
	}
}

func TestInvoiceValidate(t *testing.T) {
	base := func() *Invoice {
		return &Invoice{Number: "FV/1", Currency: "PLN", Issued: "2026-03-01",
			Seller: Party{Name: "S", Addr1: "A 1", NIP: "1234563218"},
			Buyer:  Party{Name: "B", Addr1: "B 2", NIP: "9999999999"},
			Lines:  []Line{{Name: "U", Quantity: 1, UnitPrice: 10}}}
	}
	for name, breakIt := range map[string]func(*Invoice){
		"no number":         func(i *Invoice) { i.Number = "" },
		"bad currency":      func(i *Invoice) { i.Currency = "złoty" },
		"short seller NIP":  func(i *Invoice) { i.Seller.NIP = "123" },
		"unknown country":   func(i *Invoice) { i.Buyer.NIP, i.Buyer.Country = "", "Atlantis" },
		"short buyer NIP":   func(i *Invoice) { i.Buyer.NIP = "123" },
		"nameless buyer":    func(i *Invoice) { i.Buyer.Name = " " },
		"no seller address": func(i *Invoice) { i.Seller.Addr1 = "" },
		"infinite rate":     func(i *Invoice) { i.Currency, i.Rate = "EUR", "Inf" },
		"exponent rate":     func(i *Invoice) { i.Currency, i.Rate = "EUR", "1e3" },
		"zero rate":         func(i *Invoice) { i.Currency, i.Rate = "EUR", "0.0" },
		"NaN quantity":      func(i *Invoice) { i.Lines[0].Quantity = math.NaN() },
		"infinite price":    func(i *Invoice) { i.Lines[0].UnitPrice = math.Inf(1) },
		"vanishing line":    func(i *Invoice) { i.Lines[0].Quantity = 0.0000001 },
		"NaN total":         func(i *Invoice) { i.DeclaredTotal = math.NaN() },
		"odd VAT rate":      func(i *Invoice) { i.VATRate = 7 },
		"VAT in euro":       func(i *Invoice) { i.VATRate, i.Currency, i.Rate = 23, "EUR", "4.2" },
		"VAT and exemption": func(i *Invoice) { i.VATRate, i.Exemption = 23, "art. 43" },
		"total without VAT": func(i *Invoice) { i.VATRate, i.DeclaredTotal = 23, 10 },
		"bad seller NIP":    func(i *Invoice) { i.Seller.NIP = "1234567890" },
		"letters in NIP":    func(i *Invoice) { i.Buyer.NIP = "SK2020123456" },
		"PL VAT, bad NIP":   func(i *Invoice) { i.Buyer.NIP, i.Buyer.VAT = "", "PLABCDEFGHIJ" },
		"no lines":          func(i *Invoice) { i.Lines = nil },
		"free line":         func(i *Invoice) { i.Lines[0].UnitPrice = 0 },
		"total mismatch":    func(i *Invoice) { i.DeclaredTotal = 11 },
		"rate on PLN":       func(i *Invoice) { i.Rate = "4.2" },
		"bad date":          func(i *Invoice) { i.DueDate = "15.03.2026" },
		"bad VAT number":    func(i *Invoice) { i.Buyer.VAT = "143/153" },
	} {
		inv := base()
		breakIt(inv)
		err := inv.normalise()
		if err == nil {
			err = inv.validate()
		}
		if err == nil {
			t.Errorf("%s: should be refused", name)
		}
	}
	inv := base()
	if err := inv.normalise(); err != nil || inv.validate() != nil {
		t.Errorf("the base invoice should pass: %v %v", err, inv.validate())
	}
}
