package main

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/url"
	"os"
	"strconv"
	"time"
)

type invoiceMeta struct {
	KsefNumber    string      `json:"ksefNumber"`
	InvoiceNumber string      `json:"invoiceNumber"`
	IssueDate     string      `json:"issueDate"`
	GrossAmount   json.Number `json:"grossAmount"`
	Currency      string      `json:"currency"`
	InvoicingMode string      `json:"invoicingMode"` // Online | Offline
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
					paint(os.Stderr, yellow, "[ WARN ]"),
					"over 10000 invoices in one quarter, list is cut short",
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
	fmt.Printf("Invoices issued by %s in %d (%s): %d\n", cfg.NIP, year,
		envTag(os.Stdout, cfg.Env), len(invoices))
	if len(invoices) > 0 {
		printList(os.Stdout, invoices)
	}
	return nil
}

// printList prints the invoices as a table, with late arrivals standing out.
func printList(w *os.File, invoices []invoiceMeta) {
	rows := [][]string{{"DATE", "NUMBER", "GROSS", "BUYER", "MODE", "KSEF NUMBER"}}
	for _, inv := range invoices {
		gross := inv.GrossAmount.String()
		if f, err := inv.GrossAmount.Float64(); err == nil {
			gross = strconv.FormatFloat(f, 'f', 2, 64)
		}
		buyer := []rune(inv.Buyer.Name)
		if len(buyer) > 32 {
			buyer = append(buyer[:31], '…')
		}
		rows = append(rows, []string{inv.IssueDate, inv.InvoiceNumber,
			gross + " " + inv.Currency, string(buyer), inv.InvoicingMode, inv.KsefNumber})
	}
	printTable(w, rows, 2, func(row, col int) string {
		switch {
		case row == 0:
			return bold
		case col == 4 && rows[row][col] == "Offline":
			return yellow
		case col == 4:
			return green
		case col == 5:
			return dim
		}
		return ""
	})
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
		if err := os.WriteFile(args[1], invoice, 0o644); err != nil {
			return err
		}
		fmt.Printf("%s saved %s (%s)\n", paint(os.Stdout, green, "[  OK  ]"), args[1],
			envTag(os.Stdout, cfg.Env))
		if head, err := parseInvoiceHead(invoice); err == nil {
			printSummary(os.Stdout, head)
		}
		return nil
	}
	return showInvoice(invoice, args[0]+" ("+envTag(os.Stderr, cfg.Env)+")", nil)
}
