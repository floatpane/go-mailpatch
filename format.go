package mailpatch

import (
	"fmt"
	"net/mail"
	"strings"
	"time"
)

// FormatOptions describes a patch email to be constructed.
type FormatOptions struct {
	// From is the sender address in "Name <email>" form (required).
	From string
	// To is the primary recipient address list, comma-separated (required).
	To string
	// Cc is the carbon-copy recipient list, comma-separated.
	Cc string
	// Subject is the patch subject without any [PATCH...] prefix (required).
	Subject string
	// Body is the commit message text (the prose before the diff).
	Body string
	// Diff is the raw unified diff ("diff --git ..." lines).
	Diff string
	// Version is the series revision (1 default, 2 for v2, etc.).
	Version int
	// Index is the patch position within a series (1-based). 0 means single patch.
	Index int
	// Total is the total number of patches in the series. 0 means single patch.
	Total int
	// Prefix overrides the bracket prefix token; "PATCH" by default, "RFC PATCH" for RFCs.
	Prefix string
	// InReplyTo is the Message-ID this patch replies to (for threading).
	InReplyTo string
	// References is the full References header value (space-separated Message-IDs).
	References string
	// Date overrides the Date header; if zero, time.Now() is used.
	Date time.Time
}

// Format constructs a raw RFC 5322 format-patch email from structured data.
// The returned bytes are suitable for sending via SMTP or saving as an mbox entry.
func Format(opts FormatOptions) ([]byte, error) {
	if opts.From == "" {
		return nil, fmt.Errorf("From is required")
	}
	if opts.To == "" {
		return nil, fmt.Errorf("To is required")
	}
	if opts.Subject == "" {
		return nil, fmt.Errorf("Subject is required")
	}
	if opts.Diff == "" {
		return nil, fmt.Errorf("Diff is required")
	}
	if opts.Version < 1 {
		opts.Version = 1
	}
	if opts.Prefix == "" {
		opts.Prefix = "PATCH"
	}

	date := opts.Date
	if date.IsZero() {
		date = time.Now()
	}

	addr, err := mail.ParseAddress(opts.From)
	if err != nil {
		return nil, fmt.Errorf("invalid From address: %w", err)
	}

	subject := buildPatchSubject(opts)
	messageID := generateMessageID(addr.Address)

	var hdr strings.Builder
	fmt.Fprintf(&hdr, "From: %s\r\n", formatAddress(addr))
	fmt.Fprintf(&hdr, "To: %s\r\n", opts.To)
	if opts.Cc != "" {
		fmt.Fprintf(&hdr, "Cc: %s\r\n", opts.Cc)
	}
	fmt.Fprintf(&hdr, "Subject: %s\r\n", subject)
	fmt.Fprintf(&hdr, "Date: %s\r\n", date.Format(time.RFC1123Z))
	fmt.Fprintf(&hdr, "Message-Id: <%s>\r\n", messageID)
	if opts.InReplyTo != "" {
		fmt.Fprintf(&hdr, "In-Reply-To: <%s>\r\n", trimAngles(opts.InReplyTo))
	}
	if opts.References != "" {
		refs := splitRefs(opts.References)
		var refList []string
		for _, r := range refs {
			refList = append(refList, "<"+trimAngles(r)+">")
		}
		fmt.Fprintf(&hdr, "References: %s\r\n", strings.Join(refList, " "))
	}
	hdr.WriteString("MIME-Version: 1.0\r\n")
	hdr.WriteString("Content-Type: text/plain; charset=UTF-8\r\n")
	hdr.WriteString("Content-Transfer-Encoding: 8bit\r\n")
	hdr.WriteString("\r\n")

	var body strings.Builder
	if opts.Body != "" {
		body.WriteString(opts.Body)
		body.WriteString("\n")
	}
	body.WriteString("---\n")
	body.WriteString(opts.Diff)
	if !strings.HasSuffix(opts.Diff, "\n") {
		body.WriteString("\n")
	}
	body.WriteString("-- \n")
	body.WriteString("2.49.0\n")

	raw := hdr.String() + body.String()
	return []byte(raw), nil
}

func buildPatchSubject(opts FormatOptions) string {
	var parts []string
	parts = append(parts, opts.Prefix)
	if opts.Version > 1 {
		parts = append(parts, fmt.Sprintf("v%d", opts.Version))
	}
	if opts.Index > 0 && opts.Total > 0 {
		parts = append(parts, fmt.Sprintf("%d/%d", opts.Index, opts.Total))
	}
	prefix := "[" + strings.Join(parts, " ") + "]"
	return prefix + " " + opts.Subject
}

func formatAddress(addr *mail.Address) string {
	if addr.Name != "" {
		return fmt.Sprintf("%s <%s>", addr.Name, addr.Address)
	}
	return addr.Address
}

// generateMessageID creates a simple unique Message-ID using a counter-style
// timestamp and the sender's domain.
func generateMessageID(email string) string {
	domain := "localhost"
	if idx := strings.LastIndex(email, "@"); idx >= 0 {
		domain = email[idx+1:]
	}
	return fmt.Sprintf("%d.git-send-email@%s", time.Now().UnixNano(), domain)
}

// SplitBodyDiff is the exported version of splitBodyDiff, allowing callers
// to separate a commit message from its diff without fully parsing the email.
func SplitBodyDiff(body string) (commitMsg, diff string) {
	return splitBodyDiff(body)
}
