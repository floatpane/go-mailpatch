package mailpatch

import (
	"encoding/base64"
	"errors"
	"io"
	"mime"
	"mime/multipart"
	"mime/quotedprintable"
	"net/mail"
	"regexp"
	"strconv"
	"strings"
)

// extractText returns the text/plain body of a message, decoding the
// Content-Transfer-Encoding and, for multipart messages, picking the first
// text/plain part. format-patch mail is almost always single-part text, but
// some mailers wrap it.
func extractText(h mail.Header, body io.Reader) ([]byte, error) {
	ctype := h.Get("Content-Type")
	mediatype, params, _ := mime.ParseMediaType(ctype)

	if strings.HasPrefix(mediatype, "multipart/") {
		if b := firstTextPart(body, params["boundary"]); b != nil {
			return b, nil
		}
		// Fall through: treat the whole thing as text if no part matched.
	}

	raw, err := io.ReadAll(body)
	if err != nil {
		return nil, errors.Join(ErrMalformed, err)
	}
	return decodeCTE(h.Get("Content-Transfer-Encoding"), raw), nil
}

// firstTextPart walks a multipart body and returns the decoded bytes of the
// first text/plain part, or nil if none is found.
func firstTextPart(body io.Reader, boundary string) []byte {
	if boundary == "" {
		return nil
	}
	mr := multipart.NewReader(body, boundary)
	for {
		part, err := mr.NextPart()
		if err != nil {
			return nil
		}
		mt, _, _ := mime.ParseMediaType(part.Header.Get("Content-Type"))
		if mt == "" || mt == "text/plain" {
			raw, err := io.ReadAll(part)
			if err != nil {
				return nil
			}
			return decodeCTE(part.Header.Get("Content-Transfer-Encoding"), raw)
		}
	}
}

func decodeCTE(enc string, raw []byte) []byte {
	switch strings.ToLower(strings.TrimSpace(enc)) {
	case "quoted-printable":
		if out, err := io.ReadAll(quotedprintable.NewReader(strings.NewReader(string(raw)))); err == nil {
			return out
		}
	case "base64":
		s := strings.Join(strings.Fields(string(raw)), "")
		if out, err := base64.StdEncoding.DecodeString(s); err == nil {
			return out
		}
	}
	return raw
}

// patchPrefixRe matches a run of leading "[...]" bracket groups.
var patchPrefixRe = regexp.MustCompile(`^\s*((?:\[[^\]]*\]\s*)+)(.*)$`)

// parseSubject strips a "[PATCH ...]" prefix from a subject and returns the
// clean subject plus the parsed series position. If the leading brackets do
// not look like a patch prefix (no "PATCH"/"RFC" token), the subject is
// returned unchanged.
func parseSubject(subject string) (clean string, info SeriesInfo) {
	info.Version = 1
	m := patchPrefixRe.FindStringSubmatch(subject)
	if m == nil {
		return strings.TrimSpace(subject), info
	}
	brackets, rest := m[1], m[2]

	tokens := bracketTokens(brackets)
	if !hasPatchToken(tokens) {
		// Leading brackets are something else (e.g. "[bug]"); leave as-is.
		return strings.TrimSpace(subject), info
	}

	var prefixWords []string
	for _, tok := range tokens {
		switch {
		case isCountToken(tok):
			n, total := parseCount(tok)
			info.Index, info.Total = n, total
			info.IsCover = total > 0 && n == 0
		case isVersionToken(tok):
			info.Version, _ = strconv.Atoi(tok[1:])
		default:
			prefixWords = append(prefixWords, tok)
		}
	}
	info.Prefix = strings.Join(prefixWords, " ")
	return strings.TrimSpace(rest), info
}

// bracketTokens flattens "[RFC PATCH v2 1/3]" into its whitespace-separated
// words across all leading bracket groups.
func bracketTokens(brackets string) []string {
	var toks []string
	for _, group := range regexp.MustCompile(`\[([^\]]*)\]`).FindAllStringSubmatch(brackets, -1) {
		toks = append(toks, strings.Fields(group[1])...)
	}
	return toks
}

func hasPatchToken(tokens []string) bool {
	for _, t := range tokens {
		switch strings.ToUpper(t) {
		case "PATCH", "RFC":
			return true
		}
	}
	return false
}

var countRe = regexp.MustCompile(`^(\d+)/(\d+)$`)

func isCountToken(t string) bool { return countRe.MatchString(t) }

func parseCount(t string) (n, total int) {
	m := countRe.FindStringSubmatch(t)
	n, _ = strconv.Atoi(m[1])
	total, _ = strconv.Atoi(m[2])
	return n, total
}

var versionRe = regexp.MustCompile(`^v\d+$`)

func isVersionToken(t string) bool { return versionRe.MatchString(strings.ToLower(t)) }

// splitBodyDiff separates the commit message from the diff in a format-patch
// body. The diff begins at the first "diff --git" line, or failing that at the
// first unified-diff "--- " / "+++ " header pair. The "---" separator line and
// the diffstat that git places before the diff are dropped from the body, and
// the trailing "-- \n<git version>" signature is dropped from the diff.
func splitBodyDiff(body string) (commitMsg, diff string) {
	lines := strings.Split(body, "\n")
	start := diffStart(lines)
	if start < 0 {
		return strings.TrimRight(stripSignature(lines), "\n"), ""
	}

	// Walk back over the diffstat to the "---" separator, if present, so the
	// body excludes it.
	bodyEnd := start
	if sep := separatorBefore(lines, start); sep >= 0 {
		bodyEnd = sep
	}

	commitMsg = strings.TrimRight(strings.Join(lines[:bodyEnd], "\n"), "\n")
	diffLines := lines[start:]
	diff = strings.TrimRight(trimSignatureLines(diffLines), "\n")
	return commitMsg, diff
}

func diffStart(lines []string) int {
	for i, ln := range lines {
		if strings.HasPrefix(ln, "diff --git ") {
			return i
		}
	}
	// No git header: look for a "--- " immediately followed by "+++ ".
	for i := 0; i+1 < len(lines); i++ {
		if strings.HasPrefix(lines[i], "--- ") && strings.HasPrefix(lines[i+1], "+++ ") {
			return i
		}
	}
	return -1
}

// separatorBefore finds the index of the lone "---" git separator line that
// sits between the commit message and the diffstat, searching backward from
// the diff start. Returns -1 if there is none.
func separatorBefore(lines []string, start int) int {
	for i := start - 1; i >= 0; i-- {
		if strings.TrimRight(lines[i], " \t") == "---" {
			return i
		}
	}
	return -1
}

// trimSignatureLines drops a trailing mail signature ("-- " followed by the
// git version line) from a slice of diff lines and rejoins them.
func trimSignatureLines(lines []string) string {
	for i := len(lines) - 1; i >= 0; i-- {
		if strings.TrimRight(lines[i], " \t") == "--" || lines[i] == "-- " {
			return strings.Join(lines[:i], "\n")
		}
	}
	return strings.Join(lines, "\n")
}

func stripSignature(lines []string) string {
	return trimSignatureLines(lines)
}
