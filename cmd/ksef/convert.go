package main

import (
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"maps"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"time"
)

var nbpURL = "https://api.nbp.pl/api/exchangerates/rates/a"

// convertOpts are values set on the command line, on top of what the source
// invoice says.
type convertOpts struct {
	SaleDate, Rate, RateDate, BuyerCountry, BuyerID, Unit, Exemption string
	Paid                                                             bool
	PaidDate, DueDate, Place, Payment                                string
	Send                                                             bool
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

// splitArgs splits a line into arguments the way a shell does, with single
// and double quotes: --unit "" gives an empty argument, "Nowy Sącz" one word.
func splitArgs(line string) ([]string, error) {
	var args []string
	var cur strings.Builder
	quote, open := rune(0), false
	for _, r := range line {
		switch {
		case quote != 0 && r == quote:
			quote = 0
		case quote != 0:
			cur.WriteRune(r)
		case r == '"' || r == '\'':
			quote, open = r, true
		case r == ' ' || r == '\t':
			if open {
				args = append(args, cur.String())
				cur.Reset()
				open = false
			}
		default:
			cur.WriteRune(r)
			open = true
		}
	}
	if quote != 0 {
		return nil, fmt.Errorf("unclosed %c quote", quote)
	}
	if open {
		args = append(args, cur.String())
	}
	return args, nil
}

// apply sets the options on a normalised invoice.
func (o convertOpts) apply(inv *Invoice, now time.Time) error {
	if o.BuyerCountry != "" {
		inv.Buyer.Country = strings.ToUpper(o.BuyerCountry)
	}
	if o.BuyerID != "" {
		inv.Buyer.NIP, inv.Buyer.VAT, inv.Buyer.OtherID = "", "", xmlText(o.BuyerID)
	}
	if o.DueDate != "" {
		inv.DueDate = o.DueDate
	}
	if o.Exemption != "" {
		inv.Exemption = xmlText(o.Exemption)
	}
	if o.Place != "" {
		inv.PlaceOfIssue = xmlText(o.Place)
	}
	if o.Payment != "" {
		code, ok := paymentForms[strings.ToLower(o.Payment)]
		if !ok {
			return fmt.Errorf("payment %q is not one of: %s", o.Payment,
				strings.Join(slices.Sorted(maps.Keys(paymentForms)), ", "))
		}
		inv.PaymentForm = code
	}
	for i := range inv.Lines {
		if inv.Lines[i].Unit == "" {
			inv.Lines[i].Unit = xmlText(o.Unit)
		}
	}
	issued, _ := time.Parse(time.DateOnly, inv.Issued)
	switch o.SaleDate {
	case "":
	case "prev-month-end":
		inv.SaleDate = issued.AddDate(0, 0, -issued.Day()).Format(time.DateOnly)
	default:
		inv.SaleDate = o.SaleDate
	}
	switch {
	case o.PaidDate != "":
		inv.PaidDate = o.PaidDate
	case o.Paid:
		today, err := inPoland(now)
		if err != nil {
			return err
		}
		inv.PaidDate = today
	}
	if err := inv.vatProblem(); err != nil {
		return err // before the NBP lookup below, which a refused invoice does not need
	}
	switch {
	case o.Rate != "" && o.RateDate != "":
		return errors.New("pass --rate or --rate-date, not both")
	case o.RateDate != "" && inv.Currency == "PLN":
		return errors.New("an exchange rate makes no sense for a PLN invoice")
	case o.Rate != "":
		inv.Rate = strings.ReplaceAll(o.Rate, ",", ".")
	case inv.Currency != "PLN" && inv.Rate == "":
		// the currency goes into NBP's URL, so check it before it is used
		if !isCurrency.MatchString(inv.Currency) {
			return fmt.Errorf("currency %q is not a 3-letter code", inv.Currency)
		}
		rateDate := o.RateDate
		if rateDate == "" {
			rateDate = issued.AddDate(0, 0, -1).Format(time.DateOnly)
		}
		rate, err := nbpRate(inv.Currency, rateDate)
		if err != nil {
			return fmt.Errorf("%w - or pass --rate", err)
		}
		inv.Rate = rate
	}
	return nil
}

// convertInvoify turns an Invoify export into FA(3) XML: read, normalise,
// apply the options, check, render.
func convertInvoify(in []byte, o convertOpts, now time.Time) ([]byte, error) {
	inv, err := fromInvoify(in)
	if err != nil {
		return nil, err
	}
	if err := inv.normalise(); err != nil {
		return nil, err
	}
	if err := o.apply(inv, now); err != nil {
		return nil, err
	}
	// the options bring dates of their own
	if err := inv.normalise(); err != nil {
		return nil, err
	}
	if err := inv.validate(); err != nil {
		return nil, err
	}
	return inv.render(now)
}

// convertNotes names the optional values the result leaves out.
func convertNotes(out []byte) [][2]string {
	var unset []string
	if head, err := parseInvoiceHead(out); err == nil {
		if head.SaleDate == "" && head.SaleFrom == "" {
			unset = append(unset, "sale date (--sale-date)")
		}
		if head.PaidDate == "" {
			unset = append(unset, "paid (--paid)")
		}
	}
	return [][2]string{{"not set", strings.Join(unset, ", ")}}
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
	fs.StringVar(&o.Place, "place", "", "")
	fs.StringVar(&o.Payment, "payment", "", "")
	fs.BoolVar(&o.Send, "send", false, "")
	cfg, err := loadConfig()
	if err != nil {
		return err
	}
	defaults, err := splitArgs(cfg.ConvertOptions)
	if err != nil {
		return fmt.Errorf("convert_options: %w", err)
	}
	// convert_options come first, so options on the command line win
	if err := fs.Parse(append(defaults, args...)); err != nil {
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
	if o.Send && len(files) == 1 {
		// send needs a file: the invoice goes next to its source
		files = append(files, strings.TrimSuffix(files[0], filepath.Ext(files[0]))+".xml")
	}
	if len(files) == 2 && files[0] == files[1] {
		return errors.New("OUT.xml is the same file as INVOICE - name another OUT.xml")
	}
	if len(files) == 2 {
		// the file KSeF holds must stay as it was sent
		if _, err := os.Stat(upoPathOf(files[1])); err == nil {
			return fmt.Errorf("%s was already sent (%s exists) - convert to another file",
				files[1], upoPathOf(files[1]))
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
	notes := convertNotes(out)
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
		if !o.Send {
			fmt.Println(paint(os.Stdout, dim, "Review it, then: ksef send "+files[1]+
				"  (convert --send does both)"))
			return nil
		}
		yes, err := confirm("Send it to KSeF (" + envTag(os.Stdout, cfg.Env) + ")?")
		if err != nil {
			return fmt.Errorf("%w - not sent; send it with: ksef send %s", err, files[1])
		}
		if !yes {
			fmt.Println("Not sent. Send it later with: ksef send " + files[1])
			return nil
		}
		return sendAndReport(cfg, files[1], false)
	}
	return showInvoice(out, "converted "+files[0], notes)
}
