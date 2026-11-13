package main

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/url"
	"os"
	"strconv"
	"strings"
	"time"
)

type invoiceMeta struct {
	KsefNumber    string      `json:"ksefNumber"`
	InvoiceNumber string      `json:"invoiceNumber"`
	IssueDate     string      `json:"issueDate"`
	GrossAmount   json.Number `json:"grossAmount"`
	Currency      string      `json:"currency"`
	InvoicingMode string      `json:"invoicingMode"` // Online | Offline
	Seller        struct {
		Name string `json:"name"`
	} `json:"seller"`
	Buyer struct {
		Name string `json:"name"`
	} `json:"buyer"`
}

// listInvoices returns the invoices issued from `from` up to `to` with cfg.NIP
// as the seller, or as the buyer when received is set, oldest first.
// KSeF takes at most 3 months per query, so it asks in chunks.
func listInvoices(cfg *Config, received bool, from, to, now time.Time) ([]invoiceMeta, error) {
	access, err := authenticate(cfg)
	if err != nil {
		return nil, err
	}
	subject := "Subject1"
	if received {
		subject = "Subject2"
	}
	var all []invoiceMeta
	for start := from; start.Before(to) && !start.After(now); start = start.AddDate(0, 3, 0) {
		end := start.AddDate(0, 3, 0)
		if end.After(to) {
			end = to
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
				"subjectType": subject,
				"dateRange": map[string]string{
					"dateType": "Issue",
					"from":     start.Format(time.RFC3339),
					"to":       end.Add(-time.Second).Format(time.RFC3339),
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
					"over 10000 invoices in one query, list is cut short",
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

// parsePeriod reads YEAR or YYYY-MM as a time range; empty means this year.
func parsePeriod(s string, now time.Time) (from, to time.Time, err error) {
	if s == "" {
		s = strconv.Itoa(now.Year())
	}
	if t, e := time.Parse("2006-01", s); e == nil {
		from, to = t, t.AddDate(0, 1, 0)
	} else if t, e := time.Parse("2006", s); e == nil {
		from, to = t, t.AddDate(1, 0, 0)
	} else {
		return from, to, fmt.Errorf("want YEAR or YYYY-MM, got %q", s)
	}
	if from.Year() < 2000 || from.After(now) {
		return from, to, fmt.Errorf("%s is not between 2000 and %s", s, now.Format("2006-01"))
	}
	return from, to, nil
}

func cmdList(args []string) error {
	now := time.Now().UTC()
	received, when := false, ""
	for _, a := range args {
		switch {
		case a == "--received":
			received = true
		case when == "" && !strings.HasPrefix(a, "-"):
			when = a
		default:
			return fmt.Errorf("list takes --received and one YEAR or YYYY-MM, got %q", a)
		}
	}
	from, to, err := parsePeriod(when, now)
	if err != nil {
		return err
	}
	cfg, err := loadConfig()
	if err != nil {
		return err
	}
	invoices, err := listInvoices(cfg, received, from, to, now)
	if err != nil {
		return err
	}
	what, period := "issued by", from.Format("2006")
	if received {
		what = "received by"
	}
	if to.Equal(from.AddDate(0, 1, 0)) {
		period = from.Format("2006-01")
	}
	fmt.Printf("Invoices %s %s in %s (%s): %d\n", what, cfg.NIP, period,
		envTag(os.Stdout, cfg.Env), len(invoices))
	if len(invoices) > 0 {
		printList(os.Stdout, invoices, received)
		last := invoices[len(invoices)-1].KsefNumber
		hint("Fetch one with: ksef pull " + last + " [FILE.xml]")
	}
	return nil
}

// printList prints the invoices as a table, with late arrivals standing out.
// The other party is the buyer, or the seller for received invoices.
func printList(w *os.File, invoices []invoiceMeta, received bool) {
	party := "BUYER"
	if received {
		party = "SELLER"
	}
	rows := [][]string{{"DATE", "NUMBER", "GROSS", party, "MODE", "KSEF NUMBER"}}
	for _, inv := range invoices {
		gross := inv.GrossAmount.String()
		if f, err := inv.GrossAmount.Float64(); err == nil {
			gross = strconv.FormatFloat(f, 'f', 2, 64)
		}
		name := []rune(inv.Buyer.Name)
		if received {
			name = []rune(inv.Seller.Name)
		}
		if len(name) > 32 {
			name = append(name[:31], '…')
		}
		rows = append(rows, []string{inv.IssueDate, inv.InvoiceNumber,
			gross + " " + inv.Currency, string(name), inv.InvoicingMode, inv.KsefNumber})
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
	if err := wantShape("KSeF number", args[0], isKsefNumber,
		ksefNumberForm, "ksef list shows yours"); err != nil {
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
