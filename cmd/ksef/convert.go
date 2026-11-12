package main

import (
	"bytes"
	_ "embed"
	"encoding/json"
	"encoding/xml"
	"errors"
	"flag"
	"fmt"
	"io"
	"math"
	"os"
	"regexp"
	"slices"
	"strconv"
	"strings"
	"text/template"
	"time"
	_ "time/tzdata" // invoice dates are read in Polish time, on any system
)

//go:embed fa3.xml.tmpl
var fa3Template string

var nbpURL = "https://api.nbp.pl/api/exchangerates/rates/a"

const defaultExemption = "art. 113 ust. 1 ustawy z dnia 11 marca 2004 r. " +
	"o podatku od towarów i usług"

// xmlText escapes itself when the template prints it.
type xmlText string

func (s xmlText) String() string {
	var b bytes.Buffer
	xml.EscapeText(&b, []byte(s))
	return b.String()
}

// num is a number that Invoify writes as a JSON number, a JSON string or
// XML text.
type num float64

func (n *num) UnmarshalText(b []byte) error {
	s := strings.TrimSpace(string(b))
	if s == "" || s == "null" {
		return nil
	}
	f, err := strconv.ParseFloat(s, 64)
	*n = num(f)
	return err
}

func (n *num) UnmarshalJSON(b []byte) error {
	return n.UnmarshalText(bytes.Trim(b, `"`))
}

// The xml tags serve Invoify's XML export; its JSON export matches the field
// names without them.
type invoifyParty struct {
	Name         string `xml:"name"`
	Address      string `xml:"address"`
	ZipCode      string `xml:"zipCode"`
	City         string `xml:"city"`
	Country      string `xml:"country"`
	Email        string `xml:"email"`
	Phone        string `xml:"phone"`
	CustomInputs []struct {
		Key   string `xml:"key"`
		Value string `xml:"value"`
	} `xml:"customInputs"`
}

// custom returns the first custom field found under any of the keys.
func (p invoifyParty) custom(keys ...string) string {
	for _, key := range keys {
		for _, c := range p.CustomInputs {
			if strings.EqualFold(strings.TrimSpace(c.Key), key) {
				return strings.TrimSpace(c.Value)
			}
		}
	}
	return ""
}

type invoifyAmount struct {
	Amount num `xml:"amount"`
}

type invoify struct {
	Sender   invoifyParty `xml:"sender"`
	Receiver invoifyParty `xml:"receiver"`
	Details  struct {
		InvoiceNumber   string `xml:"invoiceNumber"`
		InvoiceDate     string `xml:"invoiceDate"`
		DueDate         string `xml:"dueDate"`
		Currency        string `xml:"currency"`
		AdditionalNotes string `xml:"additionalNotes"`
		Items           []struct {
			Name      string `xml:"name"`
			Quantity  num    `xml:"quantity"`
			UnitPrice num    `xml:"unitPrice"`
		} `xml:"items"`
		TaxDetails      invoifyAmount `xml:"taxDetails"`
		DiscountDetails invoifyAmount `xml:"discountDetails"`
		ShippingDetails struct {
			Cost num `xml:"cost"`
		} `xml:"shippingDetails"`
		PaymentInformation struct {
			BankName      string `xml:"bankName"`
			AccountName   string `xml:"accountName"`
			AccountNumber string `xml:"accountNumber"`
		} `xml:"paymentInformation"`
		TotalAmount num `xml:"totalAmount"`
	} `xml:"details"`
}

type fa3Party struct {
	Country                          string
	NIP, KodUE, NrVatUE, NrID        xmlText
	Name, Addr1, Addr2, Email, Phone xmlText
}

type fa3Line struct {
	Nr                         int
	Name                       xmlText
	Quantity, UnitPrice, Total string
}

type fa3Data struct {
	Created, Issued, SaleDate, DueDate, PaidDate, Total, Rate string
	Seller, Buyer                                             fa3Party
	Currency, Number, Exemption, Unit                         xmlText
	Lines                                                     []fa3Line
	Account, BankName, AccountName, Notes, REGON              xmlText
}

// convertOpts are the values Invoify has no field for.
type convertOpts struct {
	SaleDate, Rate, RateDate, BuyerCountry, BuyerID, Unit, Exemption string
	Paid                                                             bool
	PaidDate, DueDate                                                string
}

var (
	nonDigits  = regexp.MustCompile(`\D`)
	notPhone   = regexp.MustCompile(`[^0-9+]`)
	isCountry  = regexp.MustCompile(`^[A-Z]{2}$`)
	isVatUE    = regexp.MustCompile(`^[A-Z]{2}[0-9A-Z+*]{1,12}$`)
	isCurrency = regexp.MustCompile(`^[A-Z]{3}$`)
)

// countryNames maps English and Polish names to codes, for Invoify's
// free-text country field.
var countryNames = map[string]string{
	"AUSTRIA": "AT", "BELGIUM": "BE", "BELGIA": "BE", "BULGARIA": "BG", "BUŁGARIA": "BG",
	"CROATIA": "HR", "CHORWACJA": "HR", "CYPRUS": "CY", "CYPR": "CY", "CZECHIA": "CZ",
	"CZECH REPUBLIC": "CZ", "CZECHY": "CZ", "DENMARK": "DK", "DANIA": "DK", "ESTONIA": "EE",
	"FINLAND": "FI", "FINLANDIA": "FI", "FRANCE": "FR", "FRANCJA": "FR", "GERMANY": "DE",
	"NIEMCY": "DE", "GREECE": "GR", "GRECJA": "GR", "HUNGARY": "HU", "WĘGRY": "HU",
	"IRELAND": "IE", "IRLANDIA": "IE", "ITALY": "IT", "WŁOCHY": "IT", "LATVIA": "LV",
	"ŁOTWA": "LV", "LITHUANIA": "LT", "LITWA": "LT", "LUXEMBOURG": "LU", "LUKSEMBURG": "LU",
	"MALTA": "MT", "NETHERLANDS": "NL", "HOLANDIA": "NL", "NIDERLANDY": "NL", "POLAND": "PL",
	"POLSKA": "PL", "PORTUGAL": "PT", "PORTUGALIA": "PT", "ROMANIA": "RO", "RUMUNIA": "RO",
	"SLOVAKIA": "SK", "SŁOWACJA": "SK", "SLOVENIA": "SI", "SŁOWENIA": "SI", "SPAIN": "ES",
	"HISZPANIA": "ES", "SWEDEN": "SE", "SZWECJA": "SE", "UNITED KINGDOM": "GB",
	"WIELKA BRYTANIA": "GB", "UNITED STATES": "US", "USA": "US", "STANY ZJEDNOCZONE": "US",
	"SWITZERLAND": "CH", "SZWAJCARIA": "CH", "NORWAY": "NO", "NORWEGIA": "NO",
	"UKRAINE": "UA", "UKRAINA": "UA",
}

// countryCode returns the 2-letter code for a code or a known country name.
func countryCode(s string) string {
	s = strings.ToUpper(strings.TrimSpace(s))
	if code, ok := countryNames[s]; ok {
		return code
	}
	if isCountry.MatchString(s) {
		return s
	}
	return ""
}

// inPoland is the calendar day of t in Poland.
func inPoland(t time.Time) (string, error) {
	warsaw, err := time.LoadLocation("Europe/Warsaw")
	if err != nil {
		return "", err
	}
	return t.In(warsaw).Format(time.DateOnly), nil
}

// polishDate reads an Invoify date as a calendar day in Poland. Its JSON holds
// local midnight in UTC, which is the previous day's evening; its XML holds
// "November 1, 2026".
func polishDate(s string) (string, error) {
	if t, err := time.Parse("January 2, 2006", s); err == nil {
		return t.Format(time.DateOnly), nil
	}
	if t, err := time.Parse(time.RFC3339, s); err == nil {
		return inPoland(t)
	}
	if _, err := time.Parse(time.DateOnly, s); err != nil {
		return "", fmt.Errorf("date %q: want YYYY-MM-DD", s)
	}
	return s, nil
}

func money(f float64) string {
	return strconv.FormatFloat(math.Round(f*100)/100, 'f', 2, 64)
}

func party(p invoifyParty, country string) fa3Party {
	phone := notPhone.ReplaceAllString(p.Phone, "")
	if len(phone) > 16 {
		phone = phone[:16]
	}
	return fa3Party{
		Country: country,
		Name:    xmlText(p.Name),
		Addr1:   xmlText(p.Address),
		Addr2:   xmlText(strings.TrimSpace(p.ZipCode + " " + p.City)),
		Email:   xmlText(p.Email),
		Phone:   xmlText(phone),
	}
}

// nbpRate is the NBP table A mid rate on date, or the last one before it.
func nbpRate(currency, date string) (string, error) {
	end, err := time.Parse(time.DateOnly, date)
	if err != nil {
		return "", fmt.Errorf("rate date %q: want YYYY-MM-DD", date)
	}
	// a range, so a weekend or holiday resolves to the table before it
	u := fmt.Sprintf("%s/%s/%s/%s/?format=json", nbpURL, strings.ToLower(currency),
		end.AddDate(0, 0, -10).Format(time.DateOnly), date)
	var resp struct {
		Rates []struct {
			No, EffectiveDate string
			Mid               json.Number
		}
	}
	if err := apiJSON("GET", u, "", nil, &resp); err != nil {
		return "", fmt.Errorf("NBP rate lookup: %w", err)
	}
	if len(resp.Rates) == 0 {
		return "", fmt.Errorf("NBP has no %s rate up to %s", currency, date)
	}
	r := resp.Rates[len(resp.Rates)-1]
	fmt.Fprintf(
		os.Stderr,
		"%s NBP %s rate %s (table %s, %s)\n",
		paint(os.Stderr, cyan, "[ INFO ]"),
		currency,
		r.Mid,
		r.No,
		r.EffectiveDate,
	)
	return r.Mid.String(), nil
}

// The custom fields convert reads, by party.
var (
	vatKeys      = []string{"NrVatUE", "VAT UE", "VAT-UE", "VAT ID", "VAT"}
	senderKeys   = []string{"NIP", "REGON"}
	receiverKeys = append([]string{"NIP", "NrID", "KodKraju"}, vatKeys...)
)

func decodeInvoify(in []byte) (invoify, error) {
	var inv invoify
	decode := json.Unmarshal
	if bytes.HasPrefix(bytes.TrimSpace(in), []byte("<")) {
		decode = xml.Unmarshal
	}
	return inv, decode(in, &inv)
}

// convertNotes says what the result leaves out: optional values that were not
// set, and custom fields of the source invoice that convert does not read.
func convertNotes(in, out []byte) [][2]string {
	var unset, ignored []string
	if head, err := parseInvoiceHead(out); err == nil {
		if head.SaleDate == "" && head.SaleFrom == "" {
			unset = append(unset, "sale date (--sale-date)")
		}
		if head.PaidDate == "" {
			unset = append(unset, "paid (--paid)")
		}
	}
	if inv, err := decodeInvoify(in); err == nil {
		unread := func(p invoifyParty, known []string, who string) {
			for _, c := range p.CustomInputs {
				key := strings.TrimSpace(c.Key)
				if !slices.ContainsFunc(known, func(k string) bool {
					return strings.EqualFold(k, key)
				}) {
					ignored = append(ignored, key+" ("+who+")")
				}
			}
		}
		unread(inv.Sender, senderKeys, "sender")
		unread(inv.Receiver, receiverKeys, "receiver")
	}
	return [][2]string{
		{"not set", strings.Join(unset, ", ")},
		{"ignored", strings.Join(ignored, ", ")},
	}
}

// convertInvoify turns an Invoify JSON or XML export into an FA(3) invoice of
// a VAT-exempt seller. Rated invoices, discounts and shipping are refused.
func convertInvoify(in []byte, o convertOpts, now time.Time) ([]byte, error) {
	inv, err := decodeInvoify(in)
	if err != nil {
		return nil, err
	}
	d := inv.Details
	data := fa3Data{
		Created:     now.UTC().Format("2006-01-02T15:04:05Z"),
		Number:      xmlText(d.InvoiceNumber),
		Currency:    xmlText(strings.ToUpper(strings.TrimSpace(d.Currency))),
		Exemption:   xmlText(o.Exemption),
		Unit:        xmlText(o.Unit),
		Account:     xmlText(strings.ReplaceAll(d.PaymentInformation.AccountNumber, " ", "")),
		BankName:    xmlText(d.PaymentInformation.BankName),
		AccountName: xmlText(d.PaymentInformation.AccountName),
		Notes:       xmlText(d.AdditionalNotes),
		REGON:       xmlText(inv.Sender.custom("REGON")),
	}
	if o.Exemption == "" {
		data.Exemption = defaultExemption
	}
	if d.InvoiceNumber == "" {
		return nil, errors.New("invoice number missing")
	}
	if !isCurrency.MatchString(string(data.Currency)) {
		return nil, fmt.Errorf("currency %q is not a 3-letter code", d.Currency)
	}

	sellerCountry := countryCode(inv.Sender.Country)
	if sellerCountry == "" {
		sellerCountry = "PL"
	}
	data.Seller = party(inv.Sender, sellerCountry)
	data.Seller.NIP = xmlText(nonDigits.ReplaceAllString(inv.Sender.custom("NIP"), ""))
	if len(data.Seller.NIP) != 10 {
		return nil, errors.New(`the sender needs a custom field "NIP" with a 10-digit NIP`)
	}

	recv := inv.Receiver
	buyerCountry := strings.ToUpper(o.BuyerCountry)
	if buyerCountry == "" {
		buyerCountry = countryCode(recv.custom("KodKraju"))
	}
	if buyerCountry == "" {
		buyerCountry = countryCode(recv.Country)
	}
	nip := nonDigits.ReplaceAllString(recv.custom("NIP"), "")
	vat := strings.ToUpper(strings.ReplaceAll(
		recv.custom(vatKeys...), " ", "",
	))
	if vat != "" && !isVatUE.MatchString(vat) {
		return nil, fmt.Errorf("receiver VAT number %q is not a country prefix and a number", vat)
	}
	if nip == "" && strings.HasPrefix(vat, "PL") {
		nip = vat[2:]
	}
	data.Buyer = party(recv, "")
	switch {
	case o.BuyerID != "":
		data.Buyer.NrID = xmlText(o.BuyerID)
	case nip != "":
		data.Buyer.NIP = xmlText(nip)
		if buyerCountry == "" {
			buyerCountry = "PL"
		}
	case vat != "":
		data.Buyer.KodUE, data.Buyer.NrVatUE = xmlText(vat[:2]), xmlText(vat[2:])
		if buyerCountry == "" {
			buyerCountry = vat[:2]
		}
	default:
		data.Buyer.NrID = xmlText(recv.custom("NrID"))
	}
	if !isCountry.MatchString(buyerCountry) {
		return nil, fmt.Errorf(
			"buyer country %q is not a code or a known name - pass --buyer-country", recv.Country,
		)
	}
	data.Buyer.Country = buyerCountry

	if data.Issued, err = polishDate(d.InvoiceDate); err != nil {
		return nil, fmt.Errorf("invoice date: %w", err)
	}
	if o.DueDate != "" {
		d.DueDate = o.DueDate
	}
	if d.DueDate != "" {
		if data.DueDate, err = polishDate(d.DueDate); err != nil {
			return nil, fmt.Errorf("due date: %w", err)
		}
	}
	switch o.SaleDate {
	case "":
	case "prev-month-end":
		issued, _ := time.Parse(time.DateOnly, data.Issued)
		data.SaleDate = issued.AddDate(0, 0, -issued.Day()).Format(time.DateOnly)
	default:
		if data.SaleDate, err = polishDate(o.SaleDate); err != nil {
			return nil, fmt.Errorf("sale date: %w", err)
		}
	}
	switch {
	case o.PaidDate != "":
		if data.PaidDate, err = polishDate(o.PaidDate); err != nil {
			return nil, fmt.Errorf("paid date: %w", err)
		}
	case o.Paid:
		if data.PaidDate, err = inPoland(now); err != nil {
			return nil, err
		}
	}

	if d.TaxDetails.Amount != 0 || d.DiscountDetails.Amount != 0 || d.ShippingDetails.Cost != 0 {
		return nil, errors.New("tax, discount and shipping are not supported yet")
	}
	if len(d.Items) == 0 {
		return nil, errors.New("the invoice has no items")
	}
	total := 0.0
	for i, it := range d.Items {
		if it.Name == "" || it.Quantity <= 0 || it.UnitPrice <= 0 {
			return nil, fmt.Errorf("item %d needs a name, a quantity and a unit price", i+1)
		}
		line := math.Round(float64(it.Quantity*it.UnitPrice)*100) / 100
		total += line
		data.Lines = append(data.Lines, fa3Line{
			Nr:        i + 1,
			Name:      xmlText(it.Name),
			Quantity:  strconv.FormatFloat(float64(it.Quantity), 'f', -1, 64),
			UnitPrice: money(float64(it.UnitPrice)),
			Total:     money(line),
		})
	}
	if math.Abs(total-float64(d.TotalAmount)) > 0.005 {
		return nil, fmt.Errorf("items add up to %s but the invoice total is %s",
			money(total), money(float64(d.TotalAmount)))
	}
	data.Total = money(total)

	switch {
	case o.Rate != "" && o.RateDate != "":
		return nil, errors.New("pass --rate or --rate-date, not both")
	case (o.Rate != "" || o.RateDate != "") && data.Currency == "PLN":
		return nil, errors.New("an exchange rate makes no sense for a PLN invoice")
	case o.Rate != "":
		data.Rate = strings.ReplaceAll(o.Rate, ",", ".")
		if r, err := strconv.ParseFloat(data.Rate, 64); err != nil || r <= 0 {
			return nil, fmt.Errorf("rate %q is not a positive number", o.Rate)
		}
	case data.Currency != "PLN":
		rateDate := o.RateDate
		if rateDate == "" {
			issued, _ := time.Parse(time.DateOnly, data.Issued)
			rateDate = issued.AddDate(0, 0, -1).Format(time.DateOnly)
		}
		if data.Rate, err = nbpRate(string(data.Currency), rateDate); err != nil {
			return nil, fmt.Errorf("%w - or pass --rate", err)
		}
	}

	tmpl, err := template.New("fa3").Parse(fa3Template)
	if err != nil {
		return nil, err
	}
	var out bytes.Buffer
	if err := tmpl.Execute(&out, data); err != nil {
		return nil, err
	}
	return out.Bytes(), nil
}

func cmdConvert(args []string) error {
	var o convertOpts
	fs := flag.NewFlagSet("convert", flag.ContinueOnError)
	fs.SetOutput(io.Discard)
	fs.StringVar(&o.SaleDate, "sale-date", "", "")
	fs.StringVar(&o.Rate, "rate", "", "")
	fs.StringVar(&o.RateDate, "rate-date", "", "")
	fs.StringVar(&o.BuyerCountry, "buyer-country", "", "")
	fs.StringVar(&o.BuyerID, "buyer-id", "", "")
	fs.StringVar(&o.Unit, "unit", "m-c", "")
	fs.StringVar(&o.Exemption, "exemption", "", "")
	fs.BoolVar(&o.Paid, "paid", false, "")
	fs.StringVar(&o.PaidDate, "paid-date", "", "")
	fs.StringVar(&o.DueDate, "due-date", "", "")
	cfg, err := loadConfig()
	if err != nil {
		return err
	}
	// convert_options come first, so options on the command line win
	if err := fs.Parse(append(strings.Fields(cfg.ConvertOptions), args...)); err != nil {
		return err
	}
	files := fs.Args()
	if len(files) < 1 || len(files) > 2 {
		return errors.New("convert needs INVOICE and an optional OUT.xml, after any options")
	}
	for _, f := range files {
		if strings.HasPrefix(f, "-") {
			return fmt.Errorf("options go before the files, got %q after them", f)
		}
	}
	in, err := os.ReadFile(files[0])
	if err != nil {
		return err
	}
	out, err := convertInvoify(in, o, time.Now())
	if err != nil {
		return fmt.Errorf("%s: %w", files[0], err)
	}
	notes := convertNotes(in, out)
	if len(files) == 2 {
		if err := os.WriteFile(files[1], out, 0o644); err != nil {
			return err
		}
		fmt.Printf("%s wrote %s\n", paint(os.Stdout, green, "[  OK  ]"), files[1])
		if head, err := parseInvoiceHead(out); err == nil {
			printSummary(os.Stdout, head)
		}
		for _, n := range notes {
			field(os.Stdout, n[0], n[1])
		}
		fmt.Println(paint(os.Stdout, dim, "Review it, then: ksef send "+files[1]))
		return nil
	}
	return showInvoice(out, "converted "+files[0], notes)
}
