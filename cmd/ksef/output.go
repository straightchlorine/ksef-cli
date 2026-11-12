package main

import (
	"bytes"
	"fmt"
	"os"
	"regexp"
	"runtime"
	"slices"
	"strings"
	"unicode/utf8"
)

const (
	bold, dim                = "1", "2"
	red, green, yellow, cyan = "31", "32", "33", "36"
)

func isTerminal(w *os.File) bool {
	st, err := w.Stat()
	return err == nil && st.Mode()&os.ModeCharDevice != 0
}

// colours says whether w is a terminal that takes ANSI codes.
// NO_COLOR and TERM=dumb switch them off.
func colours(w *os.File) bool {
	return isTerminal(w) && os.Getenv("NO_COLOR") == "" && os.Getenv("TERM") != "dumb" &&
		// the old Windows console prints the codes instead of colours
		(runtime.GOOS != "windows" || os.Getenv("WT_SESSION") != "")
}

func paint(w *os.File, colour, s string) string {
	if !colours(w) {
		return s
	}
	return "\x1b[" + colour + "m" + s + "\x1b[0m"
}

var (
	xmlTag  = regexp.MustCompile(`<[^>]+>`)
	xmlName = regexp.MustCompile(`^(</?)([^\s/>]+)`)
	xmlAttr = regexp.MustCompile(`([^\s=]+)=("[^"]*")`)
)

// colourXML colours tag names, attributes and attribute values, and dims the
// brackets. Text between tags is left as it is, so the values stand out.
func colourXML(b []byte) []byte {
	esc := func(colour, s string) string { return "\x1b[" + colour + "m" + s + "\x1b[0m" }
	return xmlTag.ReplaceAllFunc(b, func(tag []byte) []byte {
		s := string(tag)
		if strings.HasPrefix(s, "<?") || strings.HasPrefix(s, "<!") {
			return []byte(esc(dim, s))
		}
		end := ">"
		if strings.HasSuffix(s, "/>") {
			end = "/>"
		}
		s = strings.TrimSuffix(s, end)
		s = xmlAttr.ReplaceAllString(s, esc(yellow, "$1")+esc(dim, "=")+esc(green, "$2"))
		s = xmlName.ReplaceAllString(s, esc(dim, "$1")+esc(cyan, "$2"))
		return []byte(s + esc(dim, end))
	})
}

// showInvoice writes an invoice XML to stdout untouched, except on a terminal,
// where it is syntax-coloured. A terminal on stderr gets the status line and
// the summary after it, also when stdout goes to a file.
func showInvoice(invoice []byte, status string, notes [][2]string) error {
	out := invoice
	if colours(os.Stdout) {
		out = bytes.ReplaceAll(out, []byte("\t"), []byte("  "))
		out = colourXML(out)
	}
	if _, err := os.Stdout.Write(out); err != nil {
		return err
	}
	if !isTerminal(os.Stderr) {
		return nil
	}
	if isTerminal(os.Stdout) && !bytes.HasSuffix(out, []byte("\n")) {
		fmt.Fprintln(os.Stderr)
	}
	fmt.Fprintln(os.Stderr, paint(os.Stderr, green, "[  OK  ]"), status)
	if head, err := parseInvoiceHead(invoice); err == nil {
		printSummary(os.Stderr, head)
	}
	for _, n := range notes {
		field(os.Stderr, n[0], n[1])
	}
	return nil
}

// envTag is "env=prod", loud for production where invoices are legally binding.
func envTag(w *os.File, env string) string {
	if env == "prod" {
		return paint(w, bold+";"+yellow, "env=prod")
	}
	return paint(w, cyan, "env="+env)
}

var usageMarks = regexp.MustCompile(`(?m)^(Usage:|Subcommands:|Config|  [a-z_]+\b)`)

// printUsage prints the help with its headings and command names in bold.
func printUsage(w *os.File) {
	fmt.Fprint(w, usageMarks.ReplaceAllStringFunc(usage, func(s string) string {
		return paint(w, bold, s)
	}))
}

// joined joins the non-empty parts.
func joined(sep string, parts ...string) string {
	var kept []string
	for _, p := range parts {
		if p != "" {
			kept = append(kept, p)
		}
	}
	return strings.Join(kept, sep)
}

// prefixed is prefix+s, or nothing when s is empty.
func prefixed(prefix, s string) string {
	if s == "" {
		return ""
	}
	return prefix + s
}

// field prints one "  label:  value" line, skipped when there is no value.
func field(w *os.File, label, value string) {
	if value != "" {
		fmt.Fprintf(w, "  %s %s\n", paint(w, dim, fmt.Sprintf("%-8s", label+":")), value)
	}
}

// printSummary shows what an invoice says: number, dates, parties, total, payment.
func printSummary(w *os.File, h *invoiceHead) {
	b := h.Buyer
	buyerID := joined(" ", prefixed("NIP ", b.NIP), prefixed("VAT ", b.KodUE+b.NrVatUE),
		prefixed("id ", b.NrID))
	issued := prefixed("issued ", joined(" in ", h.IssueDate, h.Place))
	field(w, "invoice", joined(", ", h.Number, issued,
		prefixed("sale ", h.SaleDate), prefixed("sale ", joined(" to ", h.SaleFrom, h.SaleTo))))
	field(w, "seller", joined(", ", h.SellerName, prefixed("NIP ", h.SellerNIP)))
	field(w, "buyer", joined(", ", b.Name, buyerID))
	for i, l := range h.Lines {
		if i == 3 {
			field(w, "item", fmt.Sprintf("... and %d more", len(h.Lines)-i))
			break
		}
		price := joined(" x ", joined(" ", l.Quantity, l.Unit), l.Price+l.NetPrice)
		field(w, "item", joined(", ", l.Name, price))
	}
	rate := ""
	if len(h.Lines) > 0 {
		rate = h.Lines[0].Rate
	}
	field(w, "total", joined(" ", h.Total, h.Currency, prefixed("at ", rate)))
	field(w, "payment", joined(", ", prefixed("due ", h.DueDate), prefixed("paid ", h.PaidDate)))
	field(w, "order", h.Order)
	field(w, "extra", strings.Join(slices.Compact(slices.Clone(h.Extras)), ", "))
}

// printTable prints rows in aligned columns: the first row is the header,
// rightAlign names the column of numbers, and colour paints a finished cell.
func printTable(w *os.File, rows [][]string, rightAlign int, colour func(row, col int) string) {
	widths := make([]int, len(rows[0]))
	for _, row := range rows {
		for i, cell := range row {
			widths[i] = max(widths[i], utf8.RuneCountInString(cell))
		}
	}
	for r, row := range rows {
		cells := make([]string, len(row))
		for i, cell := range row {
			pad := strings.Repeat(" ", widths[i]-utf8.RuneCountInString(cell))
			if i == rightAlign {
				cell = pad + cell
			} else if i < len(row)-1 {
				cell += pad
			}
			if c := colour(r, i); c != "" {
				cell = paint(w, c, cell)
			}
			cells[i] = cell
		}
		fmt.Fprintln(w, strings.Join(cells, "  "))
	}
}
