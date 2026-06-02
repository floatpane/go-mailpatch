package mailpatch

import (
	"errors"
	"io"
	"sort"
	"strings"
)

// Series is a patch series: an optional cover letter plus the numbered
// patches, ordered by their position in the series.
type Series struct {
	// Cover is the "0/n" cover letter, or nil if the series had none.
	Cover *Patch
	// Patches are the numbered patches, sorted by SeriesInfo.Index.
	Patches []*Patch
	// Version is the series revision (1, 2, ... from "[PATCH vN ...]").
	Version int
	// Total is the expected patch count (m in "[PATCH n/m]"), 0 if unknown.
	Total int
}

// Len returns the number of numbered patches in the series.
func (s *Series) Len() int { return len(s.Patches) }

// Complete reports whether every patch in the series is present: Total is
// known and that many numbered patches were parsed.
func (s *Series) Complete() bool {
	return s.Total > 0 && len(s.Patches) == s.Total
}

// ParseMbox parses every message in an mbox stream into a Patch, in file
// order. Messages without a diff (cover letters) are included.
func ParseMbox(r io.Reader) ([]*Patch, error) {
	data, err := io.ReadAll(r)
	if err != nil {
		return nil, joinMalformed(err)
	}
	chunks := splitMbox(data)
	if len(chunks) == 0 {
		return nil, ErrEmpty
	}
	patches := make([]*Patch, 0, len(chunks))
	for _, c := range chunks {
		p, err := ParseBytes(c)
		if err != nil {
			return nil, err
		}
		patches = append(patches, p)
	}
	return patches, nil
}

// ParseSeries parses an mbox and groups it into a single Series: the cover
// letter (if any) and the numbered patches sorted by index.
func ParseSeries(r io.Reader) (*Series, error) {
	patches, err := ParseMbox(r)
	if err != nil {
		return nil, err
	}
	s := &Series{Version: 1}
	for _, p := range patches {
		if p.IsCoverLetter() {
			if s.Cover == nil {
				s.Cover = p
			}
			continue
		}
		s.Patches = append(s.Patches, p)
	}
	sort.SliceStable(s.Patches, func(i, j int) bool {
		return s.Patches[i].Series.Index < s.Patches[j].Series.Index
	})

	// Adopt version/total from whichever messages carry them.
	for _, p := range append([]*Patch{s.Cover}, s.Patches...) {
		if p == nil {
			continue
		}
		if p.Series.Version > s.Version {
			s.Version = p.Series.Version
		}
		if p.Series.Total > s.Total {
			s.Total = p.Series.Total
		}
	}
	return s, nil
}

// splitMbox divides an mbox into per-message byte slices. A message boundary
// is a line beginning with "From " at the start of input or just after a blank
// line — the standard mbox "From_" delimiter that git format-patch emits.
func splitMbox(data []byte) [][]byte {
	text := string(data)
	if strings.TrimSpace(text) == "" {
		return nil
	}
	lines := strings.SplitAfter(text, "\n")

	var (
		chunks    [][]byte
		cur       strings.Builder
		prevBlank = true // start of file counts as a boundary
	)
	flush := func() {
		if cur.Len() > 0 {
			chunks = append(chunks, []byte(cur.String()))
			cur.Reset()
		}
	}
	for _, ln := range lines {
		if prevBlank && strings.HasPrefix(ln, "From ") {
			flush()
			// Drop the From_ delimiter line itself; Parse wants headers first.
			prevBlank = false
			continue
		}
		cur.WriteString(ln)
		prevBlank = strings.TrimRight(ln, "\r\n") == ""
	}
	flush()
	return chunks
}

func joinMalformed(err error) error {
	if err == nil {
		return nil
	}
	return errors.Join(ErrMalformed, err)
}
