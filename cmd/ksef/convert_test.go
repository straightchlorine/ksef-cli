package main

import (
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"
)

// invoifyJSON is a two-line PLN invoice the way Invoify exports it: numbers as
// strings, and the date as local midnight in UTC (1 March in Poland).
func invoifyJSON(edit func(string) string) []byte {
	s := `{
  "sender": {"name": "Jan & Syn <sp. j.>", "address": "Prosta 1", "zipCode": "00-001",
    "city": "Warszawa", "country": "Poland", "email": "", "phone": "+48 600 100 200",
    "customInputs": [{"key": "NIP", "value": "123-456-32-18"}]},
  "receiver": {"name": "Kupiec", "address": "Krzywa 2", "zipCode": "00-002",
    "city": "Kraków", "country": "Polska", "email": "", "phone": "",
    "customInputs": [{"key": "nip", "value": "9999999999"}]},
  "details": {"invoiceNumber": "FV/1", "invoiceDate": "2026-02-28T23:00:00.000Z",
    "currency": "PLN",
    "items": [{"name": "Usługa", "quantity": "2", "unitPrice": "100.50"},
              {"name": "Dojazd", "quantity": 1, "unitPrice": 49}],
    "taxDetails": {"amount": 0}, "discountDetails": {"amount": 0},
    "shippingDetails": {"cost": 0}, "totalAmount": 250}
}`
	if edit != nil {
		s = edit(s)
	}
	return []byte(s)
}

var testNow = time.Date(2026, 3, 2, 10, 0, 0, 0, time.UTC)

func TestConvert(t *testing.T) {
	out, err := convertInvoify(invoifyJSON(nil), convertOpts{SaleDate: "2026-02-27"}, testNow)
	if err != nil {
		t.Fatal(err)
	}
	head, err := parseInvoiceHead(out)
	if err != nil {
		t.Fatalf("%v\n%s", err, out)
	}
	if head.SellerNIP != "1234563218" || head.IssueDate != "2026-03-01" ||
		head.Form.SystemCode != "FA (3)" {
		t.Errorf("head %+v", head)
	}
	for _, want := range []string{
		"<Nazwa>Jan &amp; Syn &lt;sp. j.&gt;</Nazwa>",
		"<NIP>9999999999</NIP>",
		"<Telefon>+48600100200</Telefon>",
		"<P_6>2026-02-27</P_6>",
		"<P_13_7>250.00</P_13_7>",
		"<P_8B>2</P_8B>",
		"<P_9B>100.50</P_9B>",
		"<P_11A>201.00</P_11A>",
		"<NrWierszaFa>2</NrWierszaFa>",
		"<P_19A>art. 113 ust. 1 ",
		"<DataWytworzeniaFa>2026-03-02T10:00:00Z</DataWytworzeniaFa>",
	} {
		if !strings.Contains(string(out), want) {
			t.Errorf("missing %s", want)
		}
	}
	for _, unwanted := range []string{"KursWaluty", "<Platnosc>", "<Stopka>", "<Email>", "<P_8A>"} {
		if strings.Contains(string(out), unwanted) {
			t.Errorf("unexpected %s", unwanted)
		}
	}
}

// A foreign buyer: country and VAT number come from the invoice, the rate from
// NBP for the day before the invoice date.
func TestConvertForeign(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/eur/2026-02-18/2026-02-28/" {
			t.Errorf("NBP path %s", r.URL.Path)
		}
		w.Write([]byte(`{"rates":[{"no":"1/A","effectiveDate":"2026-02-26","mid":4.2},
			{"no":"2/A","effectiveDate":"2026-02-27","mid":4.3955}]}`))
	}))
	defer srv.Close()
	old := nbpURL
	nbpURL = srv.URL
	defer func() { nbpURL = old }()

	foreign := func(country, custom string) []byte {
		return invoifyJSON(func(s string) string {
			s = strings.Replace(s, `"PLN"`, `"EUR"`, 1)
			s = strings.Replace(s, `"Polska"`, `"`+country+`"`, 1)
			return strings.Replace(s, `{"key": "nip", "value": "9999999999"}`, custom, 1)
		})
	}
	for name, tc := range map[string]struct {
		in   []byte
		opts convertOpts
		want []string
	}{
		"VAT number from the invoice": {
			foreign("Germany", `{"key": "VAT", "value": "DE 450511115"}`), convertOpts{},
			[]string{"<KodUE>DE</KodUE>\n      <NrVatUE>450511115</NrVatUE>",
				"<KodKraju>DE</KodKraju>\n      <AdresL1>Krzywa 2", "<KursWaluty>4.3955<"},
		},
		"buyer id and unit as options": {
			foreign("Niemcy", ""),
			convertOpts{BuyerID: "HRB 1", Unit: "szt.", RateDate: "2026-02-28"},
			[]string{"<KodKraju>DE</KodKraju>\n      <NrID>HRB 1</NrID>", "<P_8A>szt.</P_8A>"},
		},
		"explicit rate, unknown country as option": {
			foreign("Atlantis", ""), convertOpts{BuyerCountry: "xx", Rate: "4,5"},
			[]string{"<KodKraju>XX</KodKraju>", "<BrakID>1</BrakID>", "<KursWaluty>4.5<"},
		},
	} {
		out, err := convertInvoify(tc.in, tc.opts, testNow)
		if err != nil {
			t.Errorf("%s: %v", name, err)
			continue
		}
		for _, want := range tc.want {
			if !strings.Contains(string(out), want) {
				t.Errorf("%s: missing %s\n%s", name, want, out)
			}
		}
	}
	unknown := foreign("Atlantis", "")
	if _, err := convertInvoify(unknown, convertOpts{Rate: "4.5"}, testNow); err == nil {
		t.Error("an unknown country without --buyer-country should be refused")
	}
	bad := foreign("Germany", `{"key": "VAT", "value": "143/153/21302"}`)
	if _, err := convertInvoify(bad, convertOpts{Rate: "4.5"}, testNow); err == nil {
		t.Error("a VAT field that is not an EU VAT number should be refused")
	}
}

// Invoify's XML export gives the same invoice as its JSON export.
func TestConvertXMLInput(t *testing.T) {
	in := []byte(`<?xml version="1.0" encoding="UTF-8" standalone="yes"?>
<root>
  <sender><name>Jan &amp; Syn &lt;sp. j.&gt;</name><address>Prosta 1</address>
    <zipCode>00-001</zipCode><city>Warszawa</city><country>Poland</country><email/>
    <phone>+48 600 100 200</phone>
    <customInputs><key>NIP</key><value>123-456-32-18</value></customInputs></sender>
  <receiver><name>Kupiec</name><address>Krzywa 2</address><zipCode>00-002</zipCode>
    <city>Kraków</city><country>Polska</country><email/><phone/>
    <customInputs><key>nip</key><value>9999999999</value></customInputs></receiver>
  <details><invoiceNumber>FV/1</invoiceNumber><invoiceDate>March 1, 2026</invoiceDate>
    <currency>PLN</currency>
    <items><name>Usługa</name><quantity>2</quantity><unitPrice>100.50</unitPrice></items>
    <items><name>Dojazd</name><quantity>1</quantity><unitPrice>49</unitPrice></items>
    <taxDetails><amount>0</amount><taxID/></taxDetails>
    <totalAmount>250</totalAmount></details>
</root>`)
	fromXML, err := convertInvoify(in, convertOpts{}, testNow)
	if err != nil {
		t.Fatal(err)
	}
	fromJSON, err := convertInvoify(invoifyJSON(nil), convertOpts{}, testNow)
	if err != nil {
		t.Fatal(err)
	}
	if string(fromXML) != string(fromJSON) {
		t.Errorf("XML input gives\n%s\nJSON input gives\n%s", fromXML, fromJSON)
	}
}

// The sale date as the end of the previous month, and the paid date as today
// or a given day.
func TestConvertSaleAndPaidDates(t *testing.T) {
	due := func(date string) []byte {
		return invoifyJSON(func(s string) string {
			return strings.Replace(s, `"currency":`, `"dueDate": "`+date+`", "currency":`, 1)
		})
	}
	for name, tc := range map[string]struct {
		in   []byte
		opts convertOpts
		want string
	}{
		"sale date": {
			invoifyJSON(nil), convertOpts{SaleDate: "prev-month-end"}, "<P_6>2026-02-28</P_6>"},
		"paid today": {
			due("2026-03-01"), convertOpts{Paid: true}, "<DataZaplaty>2026-03-02<"},
		"due date as an option": {
			due("2026-03-15"), convertOpts{DueDate: "2026-03-20"}, "<Termin>2026-03-20</Termin>"},
		"paid on a date": {
			due("2026-03-15"), convertOpts{PaidDate: "2026-03-05"},
			"<Zaplacono>1</Zaplacono>\n      <DataZaplaty>2026-03-05<"},
	} {
		out, err := convertInvoify(tc.in, tc.opts, testNow)
		if err != nil {
			t.Errorf("%s: %v", name, err)
		} else if !strings.Contains(string(out), tc.want) {
			t.Errorf("%s: missing %s\n%s", name, tc.want, out)
		}
	}
}

// What Invoify carries beyond the basics lands on the invoice: item
// description, payment terms, order number, SWIFT and unknown custom fields.
func TestConvertCarriesExtras(t *testing.T) {
	in := invoifyJSON(func(s string) string {
		s = strings.Replace(s, `{"key": "nip", "value": "9999999999"}`,
			`{"key": "nip", "value": "9999999999"}, {"key": "Steuernummer", "value": "12/3"}`, 1)
		s = strings.Replace(s, `{"key": "NIP", "value": "123-456-32-18"}`,
			`{"key": "NIP", "value": "123-456-32-18"}, {"key": "swift", "value": "ALBPPLPW"}`, 1)
		s = strings.Replace(s, `"name": "Usługa",`, `"name": "Usługa", "description": "Opis <1>",`, 1)
		return strings.Replace(s, `"currency":`,
			`"purchaseOrderNumber": "PO-1", "paymentTerms": "7 dni", "currency":`, 1)
	})
	out, err := convertInvoify(in, convertOpts{Paid: true, Place: "Wrocław"}, testNow)
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{
		"<P_1M>Wrocław</P_1M>",
		"<NrWiersza>1</NrWiersza>\n      <Klucz>Opis</Klucz>\n      <Wartosc>Opis &lt;1&gt;</Wartosc>",
		"<Klucz>Warunki płatności</Klucz>\n      <Wartosc>7 dni</Wartosc>",
		"<Klucz>Nabywca: Steuernummer</Klucz>\n      <Wartosc>12/3</Wartosc>",
		"<NrZamowienia>PO-1</NrZamowienia>",
	} {
		if !strings.Contains(string(out), want) {
			t.Errorf("missing %s\n%s", want, out)
		}
	}
	if strings.Contains(string(out), "SWIFT") {
		t.Error("SWIFT without a bank account has nowhere to go")
	}
	want := [][2]string{{"not set", "sale date (--sale-date)"}}
	if got := convertNotes(out); !reflect.DeepEqual(got, want) {
		t.Errorf("notes %q, want %q", got, want)
	}
}

// convert_options is split like a shell line, quotes included.
func TestSplitArgs(t *testing.T) {
	for in, want := range map[string][]string{
		``:                                 nil,
		`--paid  --unit szt.`:              {"--paid", "--unit", "szt."},
		`--unit ""`:                        {"--unit", ""},
		`--place "Nowy Sącz" --paid`:       {"--place", "Nowy Sącz", "--paid"},
		`--exemption 'art. 43 "x"' --paid`: {"--exemption", `art. 43 "x"`, "--paid"},
		`--buyer-id HRB" 1"`:               {"--buyer-id", "HRB 1"},
	} {
		got, err := splitArgs(in)
		if err != nil || !reflect.DeepEqual(got, want) {
			t.Errorf("splitArgs(%q) = %q, %v; want %q", in, got, err, want)
		}
	}
	if _, err := splitArgs(`--place "Nowy`); err == nil {
		t.Error("an unclosed quote should be an error")
	}
}

// The whole command: config defaults with quotes, overridden on the command line.
func TestCmdConvert(t *testing.T) {
	dir := t.TempDir()
	in, out := filepath.Join(dir, "in.json"), filepath.Join(dir, "out.xml")
	conf := filepath.Join(dir, "ksef.conf")
	os.WriteFile(in, invoifyJSON(nil), 0o600)
	options := `convert_options=--unit "" --place "Nowy Sącz" --paid-date 2026-03-01`
	os.WriteFile(conf, []byte(options+"\n"), 0o600)
	t.Setenv("KSEF_CONFIG", conf)

	if err := cmdConvert([]string{"--paid-date", "2026-03-05", in, out}); err != nil {
		t.Fatal(err)
	}
	got, _ := os.ReadFile(out)
	for _, want := range []string{"<P_1M>Nowy Sącz</P_1M>", "<DataZaplaty>2026-03-05<"} {
		if !strings.Contains(string(got), want) {
			t.Errorf("missing %s", want)
		}
	}
	if strings.Contains(string(got), "P_8A") {
		t.Error(`--unit "" should leave the unit out`)
	}
	if err := cmdConvert([]string{in, in}); err == nil {
		t.Error("writing the result over the input should be refused")
	}
}

func TestConvertRefuses(t *testing.T) {
	replace := func(old, new string) func(string) string {
		return func(s string) string { return strings.Replace(s, old, new, 1) }
	}
	noOpts := convertOpts{}
	for name, tc := range map[string]struct {
		edit func(string) string
		opts convertOpts
	}{
		"tax": {
			replace(`"taxDetails": {"amount": 0}`, `"taxDetails": {"amount": 9}`), noOpts},
		"total mismatch":  {replace(`"totalAmount": 250`, `"totalAmount": 300`), convertOpts{}},
		"no seller NIP":   {replace(`"123-456-32-18"`, `""`), convertOpts{}},
		"rate for PLN":    {nil, convertOpts{Rate: "4.2"}},
		"bad sale date":   {nil, convertOpts{SaleDate: "27.02.2026"}},
		"rate and lookup": {replace(`"PLN"`, `"EUR"`), convertOpts{Rate: "4.2", RateDate: "2026-02-27"}},
		"bad rate":        {replace(`"PLN"`, `"EUR"`), convertOpts{Rate: "abc"}},
		"NaN quantity":    {replace(`"quantity": "2"`, `"quantity": "NaN"`), convertOpts{}},
		"infinite price":  {replace(`"unitPrice": 49`, `"unitPrice": "Inf"`), convertOpts{}},
		"NaN total":       {replace(`"totalAmount": 250`, `"totalAmount": "NaN"`), convertOpts{}},
		"hex quantity":    {replace(`"quantity": "2"`, `"quantity": "0x1p1"`), convertOpts{}},
		"foreign id as NIP": {
			replace(`"value": "9999999999"`, `"value": "SK2020123456"`), convertOpts{}},
		"NIP typo":     {replace(`"value": "9999999999"`, `"value": "9999999998"`), convertOpts{}},
		"odd currency": {replace(`"PLN"`, `"eur/../x"`), convertOpts{}},
		"blank number": {replace(`"FV/1"`, `"   "`), convertOpts{}},
	} {
		if _, err := convertInvoify(invoifyJSON(tc.edit), tc.opts, testNow); err == nil {
			t.Errorf("%s: should be refused", name)
		}
	}
}
