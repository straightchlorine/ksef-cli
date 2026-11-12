package main

import (
	"bytes"
	_ "embed"
	"encoding/xml"
	"errors"
	"fmt"
	"math"
	"math/big"
	"regexp"
	"strconv"
	"strings"
	"text/template"
	"time"
	_ "time/tzdata" // invoice dates are read in Polish time, on any system
)

//go:embed fa3.xml.tmpl
var fa3Template string

const defaultExemption = "art. 113 ust. 1 ustawy z dnia 11 marca 2004 r. " +
	"o podatku od towarów i usług"

// xmlText escapes itself when the template prints it.
type xmlText string

func (s xmlText) String() string {
	var b bytes.Buffer
	xml.EscapeText(&b, []byte(s))
	return b.String()
}

// Invoice is an invoice of a VAT-exempt seller, whatever it was read from.
// A reader fills it, normalise and validate check it, render writes FA(3).
type Invoice struct {
	Number, PlaceOfIssue, OrderNumber, Exemption xmlText
	Currency                                     string
	// YYYY-MM-DD once normalised; only Issued is required
	Issued, SaleDate, DueDate, PaidDate   string
	Rate                                  string // exchange rate of a non-PLN invoice
	Seller, Buyer                         Party
	REGON                                 xmlText
	Lines                                 []Line
	DeclaredTotal                         float64 // the source's own total, 0 for none
	Account, SWIFT, BankName, AccountName xmlText
	Footer                                xmlText
	Extras                                []Extra
	Created                               string // set by render
}

type Party struct {
	Name, Addr1, Addr2, Email, Phone xmlText
	Country                          string  // 2-letter code, or a name countryCode knows
	NIP                              string  // Polish tax id
	VAT                              string  // EU VAT number with its country prefix
	OtherID                          xmlText // any other foreign id
}

func (p Party) KodUE() string   { return p.VAT[:2] }
func (p Party) NrVatUE() string { return p.VAT[2:] }

type Line struct {
	Name, Description, Unit xmlText
	Quantity, UnitPrice     float64
}

// Qty has at most 6 decimals, the most FA(3) takes.
func (l Line) Qty() string {
	return strconv.FormatFloat(math.Round(l.Quantity*1e6)/1e6, 'f', -1, 64)
}

// Price keeps a unit price finer than a grosz, up to 8 decimals.
func (l Line) Price() string {
	p := math.Round(l.UnitPrice*1e8) / 1e8
	if math.Round(p*100)/100 == p {
		return money(p)
	}
	return strconv.FormatFloat(p, 'f', -1, 64)
}

// cents is the line sum: the printed quantity times the printed price, as
// exact decimals, rounded half up. So the three numbers on a line always agree.
func (l Line) cents() int64 {
	q, okQ := new(big.Rat).SetString(l.Qty())
	p, okP := new(big.Rat).SetString(l.Price())
	if !okQ || !okP {
		return 0
	}
	c := q.Mul(q, p)
	c.Mul(c, big.NewRat(100, 1)).Add(c, big.NewRat(1, 2))
	n := new(big.Int).Quo(c.Num(), c.Denom())
	if !n.IsInt64() {
		return 0
	}
	return n.Int64()
}

func (l Line) Sum() string { return centsText(l.cents()) }

func centsText(c int64) string { return fmt.Sprintf("%d.%02d", c/100, c%100) }

// Extra is a key/value pair shown on the invoice (DodatkowyOpis).
// Line ties it to a line number, 0 for the whole invoice.
type Extra struct {
	Line       int
	Key, Value xmlText
}

func (inv *Invoice) cents() int64 {
	var t int64
	for _, l := range inv.Lines {
		t += l.cents()
	}
	return t
}

func (inv *Invoice) Total() string { return centsText(inv.cents()) }

// Descriptions are the line descriptions followed by the extras, each value on
// one line and long ones split: FA(3) caps a value at 256 characters.
func (inv *Invoice) Descriptions() []Extra {
	all := []Extra{}
	for i, l := range inv.Lines {
		if l.Description != "" {
			all = append(all, Extra{Line: i + 1, Key: "Opis", Value: l.Description})
		}
	}
	all = append(all, inv.Extras...)
	var out []Extra
	for _, e := range all {
		key := []rune(string(e.Key))
		key = key[:min(len(key), 256)]
		for v := []rune(strings.Join(strings.Fields(string(e.Value)), " ")); len(v) > 0; {
			n := min(len(v), 256)
			out = append(out, Extra{Line: e.Line, Key: xmlText(key), Value: xmlText(v[:n])})
			v = v[n:]
		}
	}
	return out
}

var (
	nipNoise   = regexp.MustCompile(`[\s-]`)
	notPhone   = regexp.MustCompile(`[^0-9+]`)
	isCountry  = regexp.MustCompile(`^[A-Z]{2}$`)
	isVatUE    = regexp.MustCompile(`^[A-Z]{2}[0-9A-Z+*]{1,12}$`)
	isCurrency = regexp.MustCompile(`^[A-Z]{3}$`)
	isRate     = regexp.MustCompile(`^\d{1,16}(\.\d{1,6})?$`)
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

// euVATPrefixes are the country prefixes FA(3) takes for an EU VAT number.
const euVATPrefixes = "AT BE BG CY CZ DE DK EE EL ES FI FR HR HU IE IT LT LU LV MT NL PL " +
	"PT RO SE SI SK XI"

// validNIP checks the 10 digits and the check digit of a Polish tax id.
func validNIP(nip string) bool {
	if len(nip) != 10 {
		return false
	}
	sum := 0
	for i, w := range []int{6, 5, 7, 2, 3, 4, 5, 6, 7} {
		if nip[i] < '0' || nip[i] > '9' {
			return false
		}
		sum += w * int(nip[i]-'0')
	}
	return sum%11 == int(nip[9]-'0')
}

func money(f float64) string {
	return strconv.FormatFloat(math.Round(f*100)/100, 'f', 2, 64)
}

// normalise brings ids, countries, dates and the currency to their canonical
// form. A country that cannot be resolved is left as it was, for validate.
func (inv *Invoice) normalise() error {
	inv.Currency = strings.ToUpper(strings.TrimSpace(inv.Currency))
	for _, p := range []*Party{&inv.Seller, &inv.Buyer} {
		phone := notPhone.ReplaceAllString(string(p.Phone), "")
		p.Phone = xmlText(phone[:min(len(phone), 16)])
		// only separators go: a foreign number typed here must not turn into a NIP
		p.NIP = strings.TrimPrefix(strings.ToUpper(nipNoise.ReplaceAllString(p.NIP, "")), "PL")
		p.VAT = strings.ToUpper(strings.ReplaceAll(p.VAT, " ", ""))
		if p.VAT != "" && !isVatUE.MatchString(p.VAT) {
			return fmt.Errorf("VAT number %q is not a country prefix and a number", p.VAT)
		}
		switch {
		case p.VAT == "":
		case strings.HasPrefix(p.VAT, "PL"):
			if p.NIP == "" {
				p.NIP = p.VAT[2:]
			}
			p.VAT = ""
		case !strings.Contains(euVATPrefixes, p.VAT[:2]):
			// a tax id from outside the EU is an "other id" with its country
			if p.OtherID == "" {
				p.OtherID = xmlText(p.VAT)
			}
			if countryCode(p.Country) == "" {
				p.Country = p.VAT[:2]
			}
			p.VAT = ""
		}
		switch code := countryCode(p.Country); {
		case code != "":
			p.Country = code
		case p.NIP != "":
			p.Country = "PL"
		case p.VAT != "":
			p.Country = p.VAT[:2]
		}
	}
	if countryCode(inv.Seller.Country) == "" {
		inv.Seller.Country = "PL"
	}
	inv.Number = xmlText(strings.TrimSpace(string(inv.Number)))
	inv.Footer = xmlText(strings.TrimSpace(string(inv.Footer)))
	for _, d := range []struct {
		name  string
		value *string
	}{
		{"invoice date", &inv.Issued}, {"sale date", &inv.SaleDate},
		{"due date", &inv.DueDate}, {"paid date", &inv.PaidDate},
	} {
		if *d.value == "" && d.value != &inv.Issued {
			continue
		}
		day, err := polishDate(*d.value)
		if err != nil {
			return fmt.Errorf("%s: %w", d.name, err)
		}
		*d.value = day
	}
	return nil
}

func (inv *Invoice) validate() error {
	if inv.Number == "" {
		return errors.New("invoice number missing")
	}
	if !isCurrency.MatchString(inv.Currency) {
		return fmt.Errorf("currency %q is not a 3-letter code", inv.Currency)
	}
	if !validNIP(inv.Seller.NIP) {
		return fmt.Errorf("seller NIP %q is not a valid NIP", inv.Seller.NIP)
	}
	if n := inv.Buyer.NIP; n != "" && !validNIP(n) {
		return fmt.Errorf("buyer NIP %q is not a valid NIP - "+
			"a foreign number goes in the VAT or NrID field", n)
	}
	for _, p := range []struct {
		who string
		Party
	}{{"seller", inv.Seller}, {"buyer", inv.Buyer}} {
		if strings.TrimSpace(string(p.Name)) == "" || strings.TrimSpace(string(p.Addr1)) == "" {
			return fmt.Errorf("the %s needs a name and an address", p.who)
		}
	}
	if !isCountry.MatchString(inv.Buyer.Country) {
		return fmt.Errorf(
			"buyer country %q is not a code or a known name - pass --buyer-country",
			inv.Buyer.Country)
	}
	if len(inv.Lines) == 0 {
		return errors.New("the invoice has no items")
	}
	for i, l := range inv.Lines {
		// written so that NaN fails too; 1e12 keeps the arithmetic far from overflow
		if l.Name == "" || !(l.Quantity > 0 && l.Quantity < 1e12) ||
			!(l.UnitPrice > 0 && l.UnitPrice < 1e12) {
			return fmt.Errorf("item %d needs a name, a quantity and a unit price", i+1)
		}
		if l.cents() <= 0 {
			return fmt.Errorf("item %d comes to nothing: %s x %s", i+1, l.Qty(), l.Price())
		}
	}
	if d := inv.DeclaredTotal; d != 0 && !(math.Abs(float64(inv.cents())/100-d) <= 0.005) {
		return fmt.Errorf("items add up to %s but the invoice total is %v", inv.Total(), d)
	}
	if inv.Rate != "" {
		if inv.Currency == "PLN" {
			return errors.New("an exchange rate makes no sense for a PLN invoice")
		}
		// ParseFloat alone would let "Inf" and "1e3" through
		if r, _ := strconv.ParseFloat(inv.Rate, 64); !isRate.MatchString(inv.Rate) || r <= 0 {
			return fmt.Errorf("rate %q is not a positive number with up to 6 decimals", inv.Rate)
		}
	}
	return nil
}

// render writes the invoice as FA(3) XML. Call normalise and validate first.
func (inv *Invoice) render(now time.Time) ([]byte, error) {
	inv.Created = now.UTC().Format("2006-01-02T15:04:05Z")
	if inv.Exemption == "" {
		inv.Exemption = defaultExemption
	}
	inc := template.FuncMap{"inc": func(i int) int { return i + 1 }}
	tmpl, err := template.New("fa3").Funcs(inc).Parse(fa3Template)
	if err != nil {
		return nil, err
	}
	var out bytes.Buffer
	if err := tmpl.Execute(&out, inv); err != nil {
		return nil, err
	}
	return out.Bytes(), nil
}
