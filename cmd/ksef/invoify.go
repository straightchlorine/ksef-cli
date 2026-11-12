package main

import (
	"bytes"
	"cmp"
	"encoding/json"
	"encoding/xml"
	"errors"
	"fmt"
	"regexp"
	"slices"
	"strconv"
	"strings"
)

// num is a number that Invoify writes as a JSON number, a JSON string or
// XML text.
type num float64

func (n *num) UnmarshalText(b []byte) error {
	s := strings.TrimSpace(string(b))
	if s == "" || s == "null" {
		return nil
	}
	// ParseFloat alone would take "NaN", "Inf" and hex floats
	f, err := strconv.ParseFloat(s, 64)
	if err != nil || !isNumber.MatchString(s) {
		return fmt.Errorf("%q is not a number", s)
	}
	*n = num(f)
	return nil
}

var isNumber = regexp.MustCompile(`^[-+]?(\d+\.?\d*|\.\d+)([eE][-+]?\d+)?$`)

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
		InvoiceNumber       string `xml:"invoiceNumber"`
		InvoiceDate         string `xml:"invoiceDate"`
		DueDate             string `xml:"dueDate"`
		Currency            string `xml:"currency"`
		PurchaseOrderNumber string `xml:"purchaseOrderNumber"`
		PaymentTerms        string `xml:"paymentTerms"`
		AdditionalNotes     string `xml:"additionalNotes"`
		Items               []struct {
			Name        string `xml:"name"`
			Description string `xml:"description"`
			Quantity    num    `xml:"quantity"`
			UnitPrice   num    `xml:"unitPrice"`
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

// The custom fields the reader understands, by party. Any other custom field
// goes onto the invoice as an extra.
var (
	vatKeys      = []string{"NrVatUE", "VAT UE", "VAT-UE", "VAT ID", "VAT"}
	swiftKeys    = []string{"SWIFT", "BIC"}
	senderKeys   = append([]string{"NIP", "REGON", "KodKraju"}, swiftKeys...)
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

// fromInvoify reads an Invoify JSON or XML export. Rated invoices, discounts
// and shipping are refused.
func fromInvoify(in []byte) (*Invoice, error) {
	src, err := decodeInvoify(in)
	if err != nil {
		return nil, err
	}
	d := src.Details
	if d.TaxDetails.Amount != 0 || d.DiscountDetails.Amount != 0 || d.ShippingDetails.Cost != 0 {
		return nil, errors.New("tax, discount and shipping are not supported yet")
	}
	if src.Sender.custom("NIP") == "" {
		return nil, errors.New(`the sender needs a custom field "NIP" with a 10-digit NIP`)
	}
	party := func(p invoifyParty) Party {
		return Party{
			Name:    xmlText(p.Name),
			Addr1:   xmlText(p.Address),
			Addr2:   xmlText(strings.TrimSpace(p.ZipCode + " " + p.City)),
			Email:   xmlText(p.Email),
			Phone:   xmlText(p.Phone),
			Country: cmp.Or(p.custom("KodKraju"), p.Country),
			NIP:     p.custom("NIP"),
			VAT:     p.custom(vatKeys...),
			OtherID: xmlText(p.custom("NrID")),
		}
	}
	inv := &Invoice{
		Number:        xmlText(d.InvoiceNumber),
		Currency:      d.Currency,
		Issued:        d.InvoiceDate,
		DueDate:       d.DueDate,
		OrderNumber:   xmlText(strings.TrimSpace(d.PurchaseOrderNumber)),
		Seller:        party(src.Sender),
		Buyer:         party(src.Receiver),
		REGON:         xmlText(src.Sender.custom("REGON")),
		DeclaredTotal: float64(d.TotalAmount),
		Account:       xmlText(strings.ReplaceAll(d.PaymentInformation.AccountNumber, " ", "")),
		SWIFT:         xmlText(src.Sender.custom(swiftKeys...)),
		BankName:      xmlText(d.PaymentInformation.BankName),
		AccountName:   xmlText(d.PaymentInformation.AccountName),
		Footer:        xmlText(d.AdditionalNotes),
	}
	for _, it := range d.Items {
		inv.Lines = append(inv.Lines, Line{
			Name:        xmlText(it.Name),
			Description: xmlText(it.Description),
			Quantity:    float64(it.Quantity),
			UnitPrice:   float64(it.UnitPrice),
		})
	}
	if d.PaymentTerms != "" {
		inv.Extras = append(inv.Extras,
			Extra{Key: "Warunki płatności", Value: xmlText(d.PaymentTerms)})
	}
	unread := func(p invoifyParty, known []string, who string) {
		for _, c := range p.CustomInputs {
			key := strings.TrimSpace(c.Key)
			if key != "" && !slices.ContainsFunc(known, func(k string) bool {
				return strings.EqualFold(k, key)
			}) {
				inv.Extras = append(inv.Extras,
					Extra{Key: xmlText(who + ": " + key), Value: xmlText(c.Value)})
			}
		}
	}
	unread(src.Sender, senderKeys, "Sprzedawca")
	unread(src.Receiver, receiverKeys, "Nabywca")
	return inv, nil
}
