package mailpatch

import (
	"regexp"
	"strconv"
	"strings"
)

// ChangeType classifies what happened to a file in a diff.
type ChangeType int

const (
	// Modified is an in-place edit (the default).
	Modified ChangeType = iota
	// Added is a new file (old side is /dev/null).
	Added
	// Deleted is a removed file (new side is /dev/null).
	Deleted
	// Renamed is a move, possibly with edits.
	Renamed
	// Copied is a copy, possibly with edits.
	Copied
)

func (c ChangeType) String() string {
	switch c {
	case Added:
		return "added"
	case Deleted:
		return "deleted"
	case Renamed:
		return "renamed"
	case Copied:
		return "copied"
	case Modified:
		return "modified"
	default:
		return "modified"
	}
}

// FileChange is the diff for a single file.
type FileChange struct {
	OldPath  string
	NewPath  string
	Type     ChangeType
	IsBinary bool
	// OldMode and NewMode are the unix mode strings when git reports them
	// (e.g. "100644"), otherwise empty.
	OldMode string
	NewMode string
	// Additions and Deletions count added and removed lines across all hunks.
	Additions int
	Deletions int
	Hunks     []Hunk
}

// Path returns the file's current path: NewPath, or OldPath for a deletion.
func (f FileChange) Path() string {
	if f.NewPath != "" {
		return f.NewPath
	}
	return f.OldPath
}

// Hunk is one "@@ ... @@" section of a file diff.
type Hunk struct {
	OldStart int
	OldLines int
	NewStart int
	NewLines int
	// Section is the text after the closing "@@" (often the enclosing
	// function), trimmed.
	Section string
	Lines   []Line
}

// LineKind tags a diff line as context, addition, or deletion.
type LineKind int

const (
	// Context is an unchanged line (leading space).
	Context LineKind = iota
	// Add is an added line (leading '+').
	Add
	// Delete is a removed line (leading '-').
	Delete
)

// Line is one line within a hunk, with its leading +/-/space removed.
type Line struct {
	Kind LineKind
	Text string
}

// DiffStat is the summary count across a set of file changes.
type DiffStat struct {
	FilesChanged int
	Additions    int
	Deletions    int
}

var hunkRe = regexp.MustCompile(`^@@ -(\d+)(?:,(\d+))? \+(\d+)(?:,(\d+))? @@(.*)$`)

// ParseDiff parses a unified diff (git or plain) into per-file changes. It
// accepts the output of `git diff`/`git format-patch` as well as a bare
// "--- / +++ / @@" diff with no "diff --git" headers. Unrecognized lines are
// ignored, so a diff embedded in surrounding text still parses.
func ParseDiff(diff string) ([]FileChange, error) {
	var p diffParser
	for _, line := range strings.Split(diff, "\n") {
		p.consume(line)
	}
	p.flush()
	return p.files, nil
}

// diffParser holds the running state while walking a diff line by line.
type diffParser struct {
	files []FileChange
	cur   *FileChange
	hunk  *Hunk
	// oldRem/newRem are the old/new lines still expected in the current hunk,
	// from its "@@" header. They bound the hunk body so trailing blank lines
	// (e.g. the artifact of the diff's final newline) are not swallowed.
	oldRem, newRem int
}

func (p *diffParser) flush() {
	if p.cur != nil {
		p.files = append(p.files, *p.cur)
	}
	p.cur = nil
	p.hunk = nil
}

func (p *diffParser) newFile() *FileChange {
	p.flush()
	p.cur = &FileChange{Type: Modified}
	return p.cur
}

// ensure returns the current file, starting one if none is open.
func (p *diffParser) ensure() *FileChange {
	if p.cur == nil {
		return p.newFile()
	}
	return p.cur
}

func (p *diffParser) consume(line string) {
	switch {
	case p.header(line):
	case p.hunkStart(line):
	default:
		p.body(line)
	}
}

// header dispatches the file-level header lines; it returns false for anything
// that is not a header so the caller can try the hunk and body handlers.
func (p *diffParser) header(line string) bool {
	return p.gitLine(line) ||
		p.modeLine(line) ||
		p.renameCopyLine(line) ||
		p.binaryLine(line) ||
		p.pathLine(line)
}

func (p *diffParser) gitLine(line string) bool {
	if !strings.HasPrefix(line, "diff --git ") {
		return false
	}
	f := p.newFile()
	f.OldPath, f.NewPath = pathsFromGitHeader(line)
	return true
}

func (p *diffParser) modeLine(line string) bool {
	switch {
	case strings.HasPrefix(line, "new file mode "):
		f := p.ensure()
		f.Type = Added
		f.NewMode = strings.TrimSpace(strings.TrimPrefix(line, "new file mode "))
	case strings.HasPrefix(line, "deleted file mode "):
		f := p.ensure()
		f.Type = Deleted
		f.OldMode = strings.TrimSpace(strings.TrimPrefix(line, "deleted file mode "))
	case strings.HasPrefix(line, "old mode "):
		p.ensure().OldMode = strings.TrimSpace(strings.TrimPrefix(line, "old mode "))
	case strings.HasPrefix(line, "new mode "):
		p.ensure().NewMode = strings.TrimSpace(strings.TrimPrefix(line, "new mode "))
	default:
		return false
	}
	return true
}

func (p *diffParser) renameCopyLine(line string) bool {
	switch {
	case strings.HasPrefix(line, "rename from "):
		f := p.ensure()
		f.Type = Renamed
		f.OldPath = unquotePath(strings.TrimPrefix(line, "rename from "))
	case strings.HasPrefix(line, "rename to "):
		f := p.ensure()
		f.Type = Renamed
		f.NewPath = unquotePath(strings.TrimPrefix(line, "rename to "))
	case strings.HasPrefix(line, "copy from "):
		f := p.ensure()
		f.Type = Copied
		f.OldPath = unquotePath(strings.TrimPrefix(line, "copy from "))
	case strings.HasPrefix(line, "copy to "):
		f := p.ensure()
		f.Type = Copied
		f.NewPath = unquotePath(strings.TrimPrefix(line, "copy to "))
	default:
		return false
	}
	return true
}

func (p *diffParser) binaryLine(line string) bool {
	if strings.HasPrefix(line, "Binary files ") || strings.HasPrefix(line, "GIT binary patch") {
		p.ensure().IsBinary = true
		return true
	}
	return false
}

func (p *diffParser) pathLine(line string) bool {
	switch {
	case strings.HasPrefix(line, "--- "):
		// A new "---" while the current file already has hunks starts the next
		// file (handles plain diffs with no "diff --git" header).
		if p.cur == nil || len(p.cur.Hunks) > 0 {
			p.newFile()
		}
		p.hunk = nil
		path, devnull := diffPath(line, "--- ")
		p.cur.OldPath = path
		if devnull {
			p.cur.Type = Added
		}
	case strings.HasPrefix(line, "+++ "):
		f := p.ensure()
		path, devnull := diffPath(line, "+++ ")
		f.NewPath = path
		if devnull {
			f.Type = Deleted
		}
	default:
		return false
	}
	return true
}

// hunkStart handles an "@@" header. It reports the line as consumed even when
// the header does not parse, so junk that merely looks like a hunk header is
// not mistaken for body content.
func (p *diffParser) hunkStart(line string) bool {
	if !strings.HasPrefix(line, "@@ ") {
		return false
	}
	m := hunkRe.FindStringSubmatch(line)
	if m == nil {
		return true
	}
	f := p.ensure()
	h := Hunk{
		OldStart: atoi(m[1]),
		OldLines: atoiDefault(m[2], 1),
		NewStart: atoi(m[3]),
		NewLines: atoiDefault(m[4], 1),
		Section:  strings.TrimSpace(m[5]),
	}
	f.Hunks = append(f.Hunks, h)
	p.hunk = &f.Hunks[len(f.Hunks)-1]
	p.oldRem, p.newRem = h.OldLines, h.NewLines
	return true
}

func (p *diffParser) body(line string) {
	if p.hunk == nil || p.cur == nil {
		return
	}
	// Once the declared line counts are exhausted the hunk is over; anything
	// after it (a blank line, the next commit's text) is not part of the diff.
	if p.oldRem <= 0 && p.newRem <= 0 {
		p.hunk = nil
		return
	}
	switch {
	case strings.HasPrefix(line, "+"):
		p.hunk.Lines = append(p.hunk.Lines, Line{Kind: Add, Text: line[1:]})
		p.cur.Additions++
		p.newRem--
	case strings.HasPrefix(line, "-"):
		p.hunk.Lines = append(p.hunk.Lines, Line{Kind: Delete, Text: line[1:]})
		p.cur.Deletions++
		p.oldRem--
	case strings.HasPrefix(line, " "):
		p.hunk.Lines = append(p.hunk.Lines, Line{Kind: Context, Text: line[1:]})
		p.oldRem--
		p.newRem--
	case strings.HasPrefix(line, "\\"):
		// "\ No newline at end of file" — not a content line.
	case line == "":
		// A blank line with its leading space stripped: still a context line
		// while the hunk has lines left to consume.
		p.hunk.Lines = append(p.hunk.Lines, Line{Kind: Context, Text: ""})
		p.oldRem--
		p.newRem--
	}
}

// statOf computes a DiffStat over parsed file changes.
func statOf(files []FileChange) DiffStat {
	s := DiffStat{FilesChanged: len(files)}
	for _, f := range files {
		s.Additions += f.Additions
		s.Deletions += f.Deletions
	}
	return s
}

func atoi(s string) int { n, _ := strconv.Atoi(s); return n }

func atoiDefault(s string, def int) int {
	if s == "" {
		return def
	}
	return atoi(s)
}

// pathsFromGitHeader extracts old and new paths from a "diff --git a/x b/y"
// line. The "---"/"+++" lines override these when present, so this is a
// fallback (it is also the only path source for pure-rename/mode diffs that
// carry no hunks).
func pathsFromGitHeader(line string) (oldPath, newPath string) {
	rest := strings.TrimPrefix(line, "diff --git ")
	// Common case: unquoted, no spaces in paths — "a/x b/y".
	if !strings.HasPrefix(rest, "\"") {
		if i := strings.Index(rest, " b/"); i >= 0 {
			return stripABPrefix(rest[:i]), stripABPrefix(rest[i+1:])
		}
	}
	// Fall back to splitting on the midpoint for the simple symmetric case.
	fields := strings.Fields(rest)
	if len(fields) == 2 {
		return stripABPrefix(fields[0]), stripABPrefix(fields[1])
	}
	return "", ""
}

func stripABPrefix(s string) string {
	s = unquotePath(s)
	if len(s) >= 2 && (s[:2] == "a/" || s[:2] == "b/") {
		return s[2:]
	}
	return s
}

// diffPath parses a path from a "--- "/"+++ " line, stripping the marker, any
// trailing tab-separated timestamp, the a//b/ prefix, and quoting. It reports
// whether the path was /dev/null.
func diffPath(line, marker string) (path string, devnull bool) {
	s := strings.TrimPrefix(line, marker)
	if i := strings.IndexByte(s, '\t'); i >= 0 {
		s = s[:i]
	}
	s = strings.TrimSpace(s)
	if s == "/dev/null" {
		return "", true
	}
	return stripABPrefix(s), false
}

// unquotePath unquotes a git C-style quoted path ("a/\303\251.txt"); for an
// unquoted path it just trims surrounding space.
func unquotePath(s string) string {
	s = strings.TrimSpace(s)
	if len(s) >= 2 && s[0] == '"' && s[len(s)-1] == '"' {
		if unq, err := strconv.Unquote(s); err == nil {
			return unq
		}
	}
	return s
}
