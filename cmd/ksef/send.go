package main

import (
	"crypto/sha256"
	"encoding/base64"
	"encoding/xml"
	"errors"
	"fmt"
	"os"
	"strings"
	"time"
)

var qrURLs = map[string]string{
	"test": "https://qr-test.ksef.mf.gov.pl",
	"demo": "https://qr-demo.ksef.mf.gov.pl",
	"prod": "https://qr.ksef.mf.gov.pl",
}

type invoiceLine struct {
	Name     string `xml:"P_7"`
	Unit     string `xml:"P_8A"`
	Quantity string `xml:"P_8B"`
	NetPrice string `xml:"P_9A"`
	Price    string `xml:"P_9B"`
	VATRate  string `xml:"P_12"`
	Rate     string `xml:"KursWaluty"`
}

// invoiceHead is what the tool reads from an FA invoice.
type invoiceHead struct {
	Form struct {
		SystemCode    string `xml:"kodSystemowy,attr"`
		SchemaVersion string `xml:"wersjaSchemy,attr"`
		Value         string `xml:",chardata"`
	} `xml:"Naglowek>KodFormularza"`
	SellerNIP  string `xml:"Podmiot1>DaneIdentyfikacyjne>NIP"`
	SellerName string `xml:"Podmiot1>DaneIdentyfikacyjne>Nazwa"`
	Buyer      struct {
		NIP     string `xml:"NIP"`
		KodUE   string `xml:"KodUE"`
		NrVatUE string `xml:"NrVatUE"`
		NrID    string `xml:"NrID"`
		Name    string `xml:"Nazwa"`
	} `xml:"Podmiot2>DaneIdentyfikacyjne"`
	IssueDate string        `xml:"Fa>P_1"`
	Number    string        `xml:"Fa>P_2"`
	SaleDate  string        `xml:"Fa>P_6"`
	SaleFrom  string        `xml:"Fa>OkresFa>P_6_Od"`
	SaleTo    string        `xml:"Fa>OkresFa>P_6_Do"`
	Total     string        `xml:"Fa>P_15"`
	VAT1      string        `xml:"Fa>P_14_1"` // VAT at 23%, 8% and 5%
	VAT2      string        `xml:"Fa>P_14_2"`
	VAT3      string        `xml:"Fa>P_14_3"`
	Currency  string        `xml:"Fa>KodWaluty"`
	Lines     []invoiceLine `xml:"Fa>FaWiersz"`
	Place     string        `xml:"Fa>P_1M"`
	Order     string        `xml:"Fa>WarunkiTransakcji>Zamowienia>NrZamowienia"`
	Extras    []string      `xml:"Fa>DodatkowyOpis>Klucz"`
	DueDate   string        `xml:"Fa>Platnosc>TerminPlatnosci>Termin"`
	PaidDate  string        `xml:"Fa>Platnosc>DataZaplaty"`
	PayForm   string        `xml:"Fa>Platnosc>FormaPlatnosci"`
}

func parseInvoiceHead(invoice []byte) (*invoiceHead, error) {
	var h invoiceHead
	if err := xml.Unmarshal(invoice, &h); err != nil {
		return nil, err
	}
	if h.Form.SystemCode == "" || h.Form.SchemaVersion == "" || h.SellerNIP == "" {
		return nil, errors.New("not an FA invoice: KodFormularza or seller NIP missing")
	}
	return &h, nil
}

// verificationURL is the public link KSeF's own QR code carries:
// <qr host>/invoice/<seller NIP>/<P_1 as DD-MM-YYYY>/<SHA-256 of the file, base64url>.
func verificationURL(env string, h *invoiceHead, invoice []byte) (string, error) {
	issued, err := time.Parse("2006-01-02", h.IssueDate)
	if err != nil {
		return "", fmt.Errorf("invoice date P_1: %w", err)
	}
	sum := sha256.Sum256(invoice)
	return fmt.Sprintf("%s/invoice/%s/%s/%s", qrURLs[env], h.SellerNIP,
		issued.Format("02-01-2006"), base64.RawURLEncoding.EncodeToString(sum[:])), nil
}

// upoPathOf is where the receipt of an invoice file is saved. A file there
// marks the invoice as sent.
func upoPathOf(invoicePath string) string {
	return strings.TrimSuffix(invoicePath, ".xml") + ".upo.xml"
}

type sent struct {
	KsefNumber string
	Session    string // what ksef status takes
	Head       *invoiceHead
	UPOPath    string // empty when the receipt could not be saved
	VerifyURL  string
}

// sendInvoice submits one invoice file in its own interactive session and
// waits for KSeF to accept it. The UPO receipt is saved next to the file.
func sendInvoice(cfg *Config, path string) (*sent, error) {
	invoice, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	head, err := parseInvoiceHead(invoice)
	if err != nil {
		return nil, fmt.Errorf("%s: %w", path, err)
	}
	verifyURL, err := verificationURL(cfg.Env, head, invoice)
	if err != nil {
		return nil, fmt.Errorf("%s: %w", path, err)
	}
	upoPath := upoPathOf(path)
	if _, err := os.Stat(upoPath); err == nil {
		return nil, fmt.Errorf("%s exists - this invoice looks already sent", upoPath)
	}
	if cfg.NIP == "" {
		cfg.NIP = head.SellerNIP
	}

	access, err := authenticate(cfg)
	if err != nil {
		return nil, err
	}
	key, iv, encrypted, err := aesEncrypt(invoice)
	if err != nil {
		return nil, err
	}
	pub, keyID, err := publicKeyFor(cfg, "SymmetricKeyEncryption")
	if err != nil {
		return nil, err
	}
	wrappedKey, err := rsaOAEPEncrypt(pub, key)
	if err != nil {
		return nil, err
	}

	var session, sentInvoice struct {
		ReferenceNumber string `json:"referenceNumber"`
	}
	sessions := baseURLs[cfg.Env] + "/sessions"
	err = apiJSON("POST", sessions+"/online", access, map[string]any{
		"formCode": map[string]string{
			"systemCode":    head.Form.SystemCode,
			"schemaVersion": head.Form.SchemaVersion,
			"value":         strings.TrimSpace(head.Form.Value),
		},
		"encryption": map[string]string{
			"encryptedSymmetricKey": base64.StdEncoding.EncodeToString(wrappedKey),
			"initializationVector":  base64.StdEncoding.EncodeToString(iv),
			"publicKeyId":           keyID,
		},
	}, &session)
	if err != nil {
		return nil, err
	}
	online := sessions + "/online/" + session.ReferenceNumber
	err = apiJSON("POST", online+"/invoices", access, map[string]any{
		"invoiceHash":             sha256B64(invoice),
		"invoiceSize":             len(invoice),
		"encryptedInvoiceHash":    sha256B64(encrypted),
		"encryptedInvoiceSize":    len(encrypted),
		"encryptedInvoiceContent": base64.StdEncoding.EncodeToString(encrypted),
	}, &sentInvoice)
	var refused *refusedError
	if err != nil && !errors.As(err, &refused) {
		return nil, fmt.Errorf("%w - the invoice may have reached KSeF: before sending it "+
			"again, check with: ksef status %s", err, session.ReferenceNumber)
	}
	if err != nil {
		return nil, err
	}
	// The invoice is in; a failed close only delays the session's own UPO.
	if _, err := apiCall("POST", online+"/close", access, nil); err != nil {
		fmt.Fprintf(os.Stderr, "%s closing the session: %v\n", paint(os.Stderr, yellow, "[ WARN ]"), err)
	}

	var inv invoiceState
	invoiceURL := sessions + "/" + session.ReferenceNumber + "/invoices/" +
		sentInvoice.ReferenceNumber
	// The invoice is with KSeF from here on, so a failing status call must not
	// read as a failed send: retry it, and say "do not resend" if it keeps failing.
	failures := 0
	for i := 0; ; i++ {
		inv.Status.Code = 0
		err := apiJSON("GET", invoiceURL, access, nil, &inv)
		if err == nil && inv.Status.Code == 0 {
			err = errors.New("the answer carries no status")
		}
		if err == nil && !inv.Status.pending() {
			break
		}
		if err != nil {
			failures++
		}
		if i == pollAttempts || failures == 5 {
			why := "KSeF is still processing it"
			if err != nil {
				why = "its status could not be read: " + err.Error()
			}
			return nil, fmt.Errorf("sent, but %s - do not resend, check later with: "+
				"ksef status %s", why, session.ReferenceNumber)
		}
		time.Sleep(pollInterval)
	}
	if inv.Status.Code != 200 {
		return nil, fmt.Errorf("invoice rejected: %s (session %s)", inv.Status.text(),
			session.ReferenceNumber)
	}

	res := &sent{KsefNumber: inv.KsefNumber, Session: session.ReferenceNumber, Head: head,
		VerifyURL: verifyURL}
	var upo []byte
	for try := 0; try < 3; try++ {
		if upo, err = apiCall("GET", invoiceURL+"/upo", access, nil); err == nil {
			break
		}
		time.Sleep(pollInterval)
	}
	if err == nil {
		err = os.WriteFile(upoPath, upo, 0o644)
	}
	if err != nil {
		// without the UPO file nothing stops a second send of this file
		fmt.Fprintf(os.Stderr, "%s accepted as %s, but the UPO was not saved: %v\n"+
			"         do not send this file again; fetch the UPO with: ksef status %s %s\n",
			paint(os.Stderr, yellow, "[ WARN ]"), inv.KsefNumber, err,
			session.ReferenceNumber, upoPath)
	} else {
		res.UPOPath = upoPath
	}
	return res, nil
}

func cmdSend(args []string) error {
	if len(args) != 1 {
		return errors.New("send needs exactly one FILE.xml")
	}
	cfg, err := loadConfig()
	if err != nil {
		return err
	}
	return sendAndReport(cfg, args[0], true)
}

// sendAndReport sends the file and prints the outcome. The summary of the
// invoice is left out when the caller has just shown it.
func sendAndReport(cfg *Config, path string, summary bool) error {
	fmt.Printf("Sending %s to KSeF (%s)...\n", path, envTag(os.Stdout, cfg.Env))
	res, err := sendInvoice(cfg, path)
	if err != nil {
		return err
	}
	fmt.Printf("%s accepted, KSeF number %s\n", paint(os.Stdout, green, "[  OK  ]"),
		paint(os.Stdout, bold, res.KsefNumber))
	if summary {
		printSummary(os.Stdout, res.Head)
	}
	field(os.Stdout, "session", res.Session)
	field(os.Stdout, "UPO", res.UPOPath)
	field(os.Stdout, "verify", res.VerifyURL)
	return nil
}
