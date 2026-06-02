// Package mailpatch parses git "format-patch" emails into structured data.
//
// `git format-patch` turns commits into RFC 5322 email messages: the commit
// subject becomes the mail Subject (prefixed with "[PATCH n/m]"), the author
// and date become headers, the commit message becomes the body, and the diff
// follows after a "---" separator and a diffstat. `git send-email` mails those
// out; reviewers reply, and maintainers feed them back to `git am`.
//
// mailpatch reads one of those messages — or a whole mbox of them — and gives
// you the pieces without shelling out to git:
//
//   - Parse / ParseBytes decode a single message into a Patch: author, date,
//     cleaned subject, [PATCH n/m] series position, commit message body, and
//     the parsed diff (per-file hunks plus a diffstat).
//   - ParseMbox splits an mbox into one Patch per message.
//   - ParseSeries groups an mbox into a Series: the cover letter (the "0/n"
//     message) plus the numbered patches in order.
//   - ParseDiff parses a bare unified diff on its own, no email envelope.
//
// It depends only on the standard library and never executes git.
package mailpatch

import (
	"bytes"
	"errors"
	"io"
	"mime"
	"net/mail"
	"strings"
	"time"
)

// Sentinel errors. Compare with errors.Is.
var (
	// ErrEmpty is returned when the input has no message at all.
	ErrEmpty = errors.New("mailpatch: empty input")
	// ErrMalformed is returned when the input is not a parseable RFC 5322
	// message (bad headers, truncated mid-header, and similar).
	ErrMalformed = errors.New("mailpatch: malformed message")
)

// Patch is a single parsed format-patch email.
//
// A message that carries no diff — most often a "0/n" cover letter — still
// parses into a Patch; its Diff is empty and HasDiff reports false.
type Patch struct {
	// From is the raw From header (decoded from any RFC 2047 encoding).
	From string
	// AuthorName and AuthorEmail are From split into its parts, best effort.
	AuthorName  string
	AuthorEmail string
	// Date is the parsed Date header; the zero time if it was absent or
	// unparseable.
	Date time.Time

	// Subject is the subject with any "[PATCH ...]" prefix stripped.
	Subject string
	// RawSubject is the original, undecoded-prefix subject line.
	RawSubject string

	// MessageID, InReplyTo and References come from the corresponding headers
	// (angle brackets stripped). They thread a series together.
	MessageID  string
	InReplyTo  string
	References []string

	// Series is the position parsed from the subject prefix.
	Series SeriesInfo

	// Body is the commit message: everything between the headers and the
	// diffstat/diff separator.
	Body string

	// Diff is the raw unified diff text, signature stripped. Empty for a
	// cover letter.
	Diff string
	// Files is Diff parsed into per-file changes.
	Files []FileChange
	// Stat is the diffstat computed from Files.
	Stat DiffStat

	// Header is the full set of decoded message headers, for callers that
	// need a field this struct does not surface.
	Header mail.Header
}

// HasDiff reports whether the message carried an actual diff.
func (p *Patch) HasDiff() bool { return p.Diff != "" }

// IsCoverLetter reports whether this is a series cover letter: a "0/n" subject
// prefix, or simply a patch mail with no diff.
func (p *Patch) IsCoverLetter() bool {
	return p.Series.IsCover || (!p.HasDiff() && p.Series.Total > 0)
}

// SeriesInfo is the position of a patch within a series, parsed from the
// "[PATCH n/m]" (or "[RFC PATCH v2 n/m]") subject prefix.
type SeriesInfo struct {
	// Index is n in "[PATCH n/m]"; 0 for a cover letter or a lone patch with
	// no "n/m".
	Index int
	// Total is m in "[PATCH n/m]"; 0 when the subject had no count.
	Total int
	// Version is the revision: 2 for "[PATCH v2 ...]", 1 when unspecified.
	Version int
	// Prefix is the prefix words other than the version and count, e.g.
	// "PATCH" or "RFC PATCH".
	Prefix string
	// IsCover is true for the "0/m" message.
	IsCover bool
}

// Parse decodes a single format-patch email from r.
func Parse(r io.Reader) (*Patch, error) {
	if r == nil {
		return nil, ErrEmpty
	}
	msg, err := mail.ReadMessage(r)
	if err != nil {
		if errors.Is(err, io.EOF) {
			return nil, ErrEmpty
		}
		return nil, errors.Join(ErrMalformed, err)
	}

	body, err := extractText(msg.Header, msg.Body)
	if err != nil {
		return nil, err
	}

	p := &Patch{Header: msg.Header}
	p.From = decodeHeader(msg.Header.Get("From"))
	p.AuthorName, p.AuthorEmail = splitAddress(p.From)
	if t, err := msg.Header.Date(); err == nil {
		p.Date = t
	}
	p.MessageID = trimAngles(msg.Header.Get("Message-ID"))
	p.InReplyTo = trimAngles(msg.Header.Get("In-Reply-To"))
	p.References = splitRefs(msg.Header.Get("References"))

	p.RawSubject = decodeHeader(msg.Header.Get("Subject"))
	p.Subject, p.Series = parseSubject(p.RawSubject)

	p.Body, p.Diff = splitBodyDiff(string(body))
	if p.Diff != "" {
		p.Files, _ = ParseDiff(p.Diff)
		p.Stat = statOf(p.Files)
	}
	return p, nil
}

// ParseBytes is Parse over an in-memory message.
func ParseBytes(b []byte) (*Patch, error) {
	return Parse(bytes.NewReader(b))
}

// decodeHeader decodes any RFC 2047 encoded-words in a header value.
func decodeHeader(v string) string {
	if v == "" {
		return ""
	}
	dec := mime.WordDecoder{}
	if out, err := dec.DecodeHeader(v); err == nil {
		return out
	}
	return v
}

func trimAngles(s string) string {
	s = strings.TrimSpace(s)
	s = strings.TrimPrefix(s, "<")
	s = strings.TrimSuffix(s, ">")
	return s
}

func splitRefs(s string) []string {
	fields := strings.Fields(s)
	if len(fields) == 0 {
		return nil
	}
	out := make([]string, 0, len(fields))
	for _, f := range fields {
		out = append(out, trimAngles(f))
	}
	return out
}

// splitAddress splits a From value into display name and email, best effort.
func splitAddress(from string) (name, email string) {
	if from == "" {
		return "", ""
	}
	if addr, err := mail.ParseAddress(from); err == nil {
		return addr.Name, addr.Address
	}
	// Fall back to a bare "Name <addr>" or "addr" split.
	if i := strings.LastIndexByte(from, '<'); i >= 0 {
		name = strings.TrimSpace(from[:i])
		email = trimAngles(from[i:])
		return name, email
	}
	return "", strings.TrimSpace(from)
}
