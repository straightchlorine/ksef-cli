package main

import (
	"errors"
	"fmt"
	"maps"
	"net/url"
	"os"
	"slices"
	"strings"
)

// invoiceState is what KSeF says about one invoice sent in a session.
type invoiceState struct {
	ReferenceNumber string     `json:"referenceNumber"`
	InvoiceNumber   string     `json:"invoiceNumber"`
	KsefNumber      string     `json:"ksefNumber"`
	Status          statusInfo `json:"status"`
}

// pending says KSeF has the invoice and is still working on it.
func (s statusInfo) pending() bool { return s.Code == 100 || s.Code == 150 }

// text is the status on one line, with the extras KSeF adds to some failures.
func (s statusInfo) text() string {
	out := fmt.Sprintf("%d %s", s.Code, s.Description)
	if len(s.Details) > 0 {
		out += fmt.Sprint(" ", s.Details)
	}
	for _, k := range slices.Sorted(maps.Keys(s.Extensions)) {
		// a duplicate's details already name the original invoice and session
		if v := s.Extensions[k]; !strings.Contains(out, v) {
			out += fmt.Sprintf(" %s=%s", k, v)
		}
	}
	return out
}

// cmdStatus shows what became of the invoices sent in a session, for a send
// that ended before KSeF gave its verdict. With UPO_FILE it saves the receipt.
func cmdStatus(args []string) error {
	if len(args) < 1 || len(args) > 2 {
		return errors.New("status needs SESSION and an optional UPO_FILE")
	}
	if err := wantShape("session number", args[0], isReference,
		referenceForm, "ksef send prints it"); err != nil {
		return err
	}
	cfg, err := loadConfig()
	if err != nil {
		return err
	}
	access, err := authenticate(cfg)
	if err != nil {
		return err
	}
	session := baseURLs[cfg.Env] + "/sessions/" + url.PathEscape(args[0])
	var list struct {
		Invoices []invoiceState `json:"invoices"`
	}
	if err := apiJSON("GET", session+"/invoices?pageSize=100", access, nil, &list); err != nil {
		return err
	}
	if len(list.Invoices) == 0 {
		var s struct {
			Status statusInfo `json:"status"`
		}
		if err := apiJSON("GET", session, access, nil, &s); err != nil {
			return err
		}
		return fmt.Errorf("no invoice reached this session (%s)", s.Status.text())
	}

	waiting, rejected := 0, 0
	for _, inv := range list.Invoices {
		switch {
		case inv.Status.pending():
			waiting++
			fmt.Printf("%s %s still processing (%s)\n", paint(os.Stdout, cyan, "[ WAIT ]"),
				inv.InvoiceNumber, inv.Status.text())
		case inv.Status.Code == 200:
			fmt.Printf("%s %s accepted, KSeF number %s\n", paint(os.Stdout, green, "[  OK  ]"),
				inv.InvoiceNumber, paint(os.Stdout, bold, inv.KsefNumber))
		default:
			rejected++
			fmt.Printf("%s %s rejected: %s\n", paint(os.Stdout, red, "[ FAIL ]"),
				inv.InvoiceNumber, inv.Status.text())
		}
	}
	switch {
	case rejected > 0:
		return fmt.Errorf("%d of %d rejected", rejected, len(list.Invoices))
	case waiting > 0:
		return fmt.Errorf("%d of %d still processing - check again in a while",
			waiting, len(list.Invoices))
	}
	if len(args) == 1 {
		if len(list.Invoices) == 1 {
			hint("Save its receipt with: ksef status " + args[0] + " FILE.upo.xml")
		}
		return nil
	}
	if len(list.Invoices) != 1 {
		return fmt.Errorf("the session holds %d invoices, one UPO_FILE does not fit them",
			len(list.Invoices))
	}
	upo, err := apiCall("GET", session+"/invoices/"+
		url.PathEscape(list.Invoices[0].ReferenceNumber)+"/upo", access, nil)
	if err != nil {
		return err
	}
	// never over an existing file: a UPO next to an invoice marks it as sent
	f, err := os.OpenFile(args[1], os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o644)
	if err != nil {
		return err
	}
	if _, err := f.Write(upo); err != nil {
		f.Close()
		return err
	}
	if err := f.Close(); err != nil {
		return err
	}
	field(os.Stdout, "UPO", args[1])
	return nil
}
