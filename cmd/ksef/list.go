package main

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/url"
	"os"
	"strconv"
	"text/tabwriter"
	"time"
)

type invoiceMeta struct {
	KsefNumber    string      `json:"ksefNumber"`
	InvoiceNumber string      `json:"invoiceNumber"`
	IssueDate     string      `json:"issueDate"`
	GrossAmount   json.Number `json:"grossAmount"`
	Currency      string      `json:"currency"`
	Buyer         struct {
		Name string `json:"name"`
	} `json:"buyer"`
}

// listInvoices returns the invoices issued in year with cfg.NIP as the seller,
// oldest first. Asks quater by quater.
func listInvoices(cfg *Config, year int, now time.Time) ([]invoiceMeta, error) {
	access, err := authenticate(cfg)
	if err != nil {
		return nil, err
	}
	var all []invoiceMeta
	for q := 0; q < 4; q++ {
		from := time.Date(year, time.Month(1+3*q), 1, 0, 0, 0, 0, time.UTC)
		to := from.AddDate(0, 3, 0).Add(-time.Second)
		if from.After(now) {
			break
		}
		for page := 0; ; page++ {
			var resp struct {
				HasMore     bool          `json:"hasMore"`
				IsTruncated bool          `json:"isTruncated"`
				Invoices    []invoiceMeta `json:"invoices"`
			}
			u := fmt.Sprintf("%s/invoices/query/metadata?sortOrder=Asc&pageSize=250&pageOffset=%d",
				baseURLs[cfg.Env], page)
			err := apiJSON("POST", u, access, map[string]any{
				"subjectType": "Subject1",
				"dateRange": map[string]string{
					"dateType": "Issue",
					"from":     from.Format(time.RFC3339),
					"to":       to.Format(time.RFC3339),
				},
			}, &resp)
			if err != nil {
				return nil, err
			}
			all = append(all, resp.Invoices...)
			if resp.IsTruncated {
				// TODO: narrow the date range and continue, as the API docs say
				fmt.Fprintln(
					os.Stderr,
					"[ WARN ] over 10000 invoices in one quarter, list is cut short",
				)
				break
			}
			if !resp.HasMore {
				break
			}
		}
	}
	return all, nil
}

func cmdList(args []string) error {
	now := time.Now().UTC()
	year := now.Year()
	if len(args) > 1 {
		return errors.New("list takes at most one YEAR")
	}
	if len(args) == 1 {
		y, err := strconv.Atoi(args[0])
		if err != nil || y < 2000 || y > now.Year() {
			return fmt.Errorf("YEAR must be between 2000 and %d, got %q", now.Year(), args[0])
		}
		year = y
	}
	cfg, err := loadConfig()
	if err != nil {
		return err
	}
	invoices, err := listInvoices(cfg, year, now)
	if err != nil {
		return err
	}
	fmt.Printf("Invoices issued by %s in %d (env=%s): %d\n", cfg.NIP, year, cfg.Env, len(invoices))
	w := tabwriter.NewWriter(os.Stdout, 0, 0, 2, ' ', 0)
	for _, inv := range invoices {
		fmt.Fprintf(w, "%s\t%s\t%s %s\t%s\t%s\n", inv.IssueDate, inv.InvoiceNumber,
			inv.GrossAmount, inv.Currency, inv.Buyer.Name, inv.KsefNumber)
	}
	return w.Flush()
}

// cmdPull fetches an invoice as KSeF stored it, to stdout or to a file.
func cmdPull(args []string) error {
	if len(args) < 1 || len(args) > 2 {
		return errors.New("pull needs KSEF_NUMBER and an optional OUT_FILE")
	}
	cfg, err := loadConfig()
	if err != nil {
		return err
	}
	access, err := authenticate(cfg)
	if err != nil {
		return err
	}
	u := baseURLs[cfg.Env] + "/invoices/ksef/" + url.PathEscape(args[0])
	invoice, err := apiCall("GET", u, access, nil)
	if err != nil {
		return err
	}
	if len(args) == 2 {
		return os.WriteFile(args[1], invoice, 0o644)
	}
	_, err = os.Stdout.Write(invoice)
	return err
}
