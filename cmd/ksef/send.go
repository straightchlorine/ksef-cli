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

// invoiceHead is the little that send needs to read from an FA invoice.
type invoiceHead struct {
	Form struct {
		SystemCode    string `xml:"kodSystemowy,attr"`
		SchemaVersion string `xml:"wersjaSchemy,attr"`
		Value         string `xml:",chardata"`
	} `xml:"Naglowek>KodFormularza"`
	SellerNIP string `xml:"Podmiot1>DaneIdentyfikacyjne>NIP"`
	IssueDate string `xml:"Fa>P_1"`
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

type sent struct {
	KsefNumber string
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
	upoPath := strings.TrimSuffix(path, ".xml") + ".upo.xml"
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
	if err != nil {
		return nil, err
	}
	// The invoice is in; a failed close only delays the session's own UPO.
	if _, err := apiCall("POST", online+"/close", access, nil); err != nil {
		fmt.Fprintf(os.Stderr, "%s closing the session: %v\n", paint(os.Stderr, yellow, "[ WARN ]"), err)
	}

	var inv struct {
		KsefNumber string     `json:"ksefNumber"`
		Status     statusInfo `json:"status"`
	}
	invoiceURL := sessions + "/" + session.ReferenceNumber + "/invoices/" +
		sentInvoice.ReferenceNumber
	for i := 0; ; i++ {
		if err := apiJSON("GET", invoiceURL, access, nil, &inv); err != nil {
			return nil, err
		}
		if inv.Status.Code != 100 && inv.Status.Code != 150 {
			break
		}
		if i == pollAttempts {
			return nil, fmt.Errorf(
				"sent, but KSeF is still processing it (session %s, invoice %s) - do not resend",
				session.ReferenceNumber, sentInvoice.ReferenceNumber,
			)
		}
		time.Sleep(pollInterval)
	}
	if inv.Status.Code != 200 {
		extra := ""
		if len(inv.Status.Extensions) > 0 {
			extra = fmt.Sprint(" ", inv.Status.Extensions)
		}
		return nil, fmt.Errorf("invoice rejected: %d %s %v%s", inv.Status.Code,
			inv.Status.Description, inv.Status.Details, extra)
	}

	res := &sent{KsefNumber: inv.KsefNumber, VerifyURL: verifyURL}
	upo, err := apiCall("GET", invoiceURL+"/upo", access, nil)
	if err == nil {
		err = os.WriteFile(upoPath, upo, 0o644)
	}
	if err != nil {
		fmt.Fprintf(os.Stderr, "%s accepted, but the UPO was not saved: %v\n",
			paint(os.Stderr, yellow, "[ WARN ]"), err)
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
	fmt.Printf("Sending %s to KSeF (env=%s)...\n", args[0], cfg.Env)
	res, err := sendInvoice(cfg, args[0])
	if err != nil {
		return err
	}
	fmt.Printf("%s accepted, KSeF number %s\n", paint(os.Stdout, green, "[  OK  ]"), res.KsefNumber)
	if res.UPOPath != "" {
		fmt.Printf("  UPO:    %s\n", res.UPOPath)
	}
	fmt.Printf("  verify: %s\n", res.VerifyURL)
	return nil
}
