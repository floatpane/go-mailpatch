package mailpatch_test

import (
	"strings"
	"testing"

	"github.com/floatpane/go-mailpatch"
)

// A complete single-commit format-patch email, as `git format-patch` emits it.
const singlePatch = `From 9f8e7d6c5b4a39281706f5e4d3c2b1a098765432 Mon Sep 17 00:00:00 2001
From: Ada Lovelace <ada@example.com>
Date: Mon, 2 Jun 2025 11:30:00 +0000
Subject: [PATCH 2/3] parser: handle empty input

The parser panicked on an empty reader. Return ErrEmpty instead so
callers can branch on it.

Signed-off-by: Ada Lovelace <ada@example.com>
---
 parser.go | 5 +++++
 1 file changed, 5 insertions(+)

diff --git a/parser.go b/parser.go
index 1234567..89abcde 100644
--- a/parser.go
+++ b/parser.go
@@ -10,6 +10,11 @@ package parser

 func Parse(r io.Reader) error {
+	if r == nil {
+		return ErrEmpty
+	}
 	// existing logic
 	return nil
 }
--
2.45.1
`

func TestParseSingle(t *testing.T) {
	p, err := mailpatch.ParseBytes([]byte(singlePatch))
	if err != nil {
		t.Fatalf("ParseBytes: %v", err)
	}

	if p.AuthorName != "Ada Lovelace" {
		t.Errorf("AuthorName = %q, want Ada Lovelace", p.AuthorName)
	}
	if p.AuthorEmail != "ada@example.com" {
		t.Errorf("AuthorEmail = %q", p.AuthorEmail)
	}
	if p.Date.IsZero() {
		t.Error("Date not parsed")
	}
	if p.Subject != "parser: handle empty input" {
		t.Errorf("Subject = %q", p.Subject)
	}
	if got := p.Series; got.Index != 2 || got.Total != 3 || got.Version != 1 {
		t.Errorf("Series = %+v, want index 2 total 3 v1", got)
	}
	if p.Series.IsCover {
		t.Error("IsCover should be false for 2/3")
	}
	if !strings.Contains(p.Body, "panicked on an empty reader") {
		t.Errorf("Body missing commit message: %q", p.Body)
	}
	if strings.Contains(p.Body, "diff --git") {
		t.Error("Body leaked the diff")
	}
	if strings.Contains(p.Body, "1 file changed") {
		t.Error("Body leaked the diffstat")
	}
	if strings.Contains(p.Diff, "2.45.1") {
		t.Error("Diff kept the mail signature")
	}

	if !p.HasDiff() {
		t.Fatal("HasDiff = false")
	}
	if len(p.Files) != 1 {
		t.Fatalf("Files = %d, want 1", len(p.Files))
	}
	f := p.Files[0]
	if f.Path() != "parser.go" {
		t.Errorf("Path = %q", f.Path())
	}
	if f.Type != mailpatch.Modified {
		t.Errorf("Type = %v, want modified", f.Type)
	}
	if f.Additions != 3 || f.Deletions != 0 {
		t.Errorf("file +%d -%d, want +3 -0", f.Additions, f.Deletions)
	}
	if p.Stat.FilesChanged != 1 || p.Stat.Additions != 3 {
		t.Errorf("Stat = %+v", p.Stat)
	}
}

func TestSubjectPrefixes(t *testing.T) {
	cases := []struct {
		subject string
		clean   string
		index   int
		total   int
		version int
		cover   bool
		prefix  string
	}{
		{"[PATCH] lone patch", "lone patch", 0, 0, 1, false, "PATCH"},
		{"[PATCH 1/4] first", "first", 1, 4, 1, false, "PATCH"},
		{"[PATCH v3 2/4] second", "second", 2, 4, 3, false, "PATCH"},
		{"[RFC PATCH 0/2] cover", "cover", 0, 2, 1, true, "RFC PATCH"},
		{"[PATCH v2] no count", "no count", 0, 0, 2, false, "PATCH"},
		{"not a patch subject", "not a patch subject", 0, 0, 1, false, ""},
		{"[bug] still not a patch", "[bug] still not a patch", 0, 0, 1, false, ""},
	}
	for _, c := range cases {
		t.Run(c.subject, func(t *testing.T) {
			// Build a minimal message with just the subject.
			msg := "Subject: " + c.subject + "\n\nbody\n"
			p, err := mailpatch.ParseBytes([]byte(msg))
			if err != nil {
				t.Fatal(err)
			}
			if p.Subject != c.clean {
				t.Errorf("Subject = %q, want %q", p.Subject, c.clean)
			}
			s := p.Series
			if s.Index != c.index || s.Total != c.total || s.Version != c.version {
				t.Errorf("Series = %+v, want index %d total %d v%d", s, c.index, c.total, c.version)
			}
			if s.IsCover != c.cover {
				t.Errorf("IsCover = %v, want %v", s.IsCover, c.cover)
			}
			if s.Prefix != c.prefix {
				t.Errorf("Prefix = %q, want %q", s.Prefix, c.prefix)
			}
		})
	}
}

const renameDelete = `diff --git a/old.txt b/new.txt
similarity index 95%
rename from old.txt
rename to new.txt
index abc..def 100644
--- a/old.txt
+++ b/new.txt
@@ -1,3 +1,3 @@
 keep
-was
+now
 keep
diff --git a/gone.txt b/gone.txt
deleted file mode 100644
index 111..0000000
--- a/gone.txt
+++ /dev/null
@@ -1,2 +0,0 @@
-line one
-line two
diff --git a/added.txt b/added.txt
new file mode 100644
index 0000000..222
--- /dev/null
+++ b/added.txt
@@ -0,0 +1,1 @@
+brand new
`

func TestParseDiffTypes(t *testing.T) {
	files, err := mailpatch.ParseDiff(renameDelete)
	if err != nil {
		t.Fatal(err)
	}
	if len(files) != 3 {
		t.Fatalf("files = %d, want 3", len(files))
	}

	rn := files[0]
	if rn.Type != mailpatch.Renamed {
		t.Errorf("file0 type = %v, want renamed", rn.Type)
	}
	if rn.OldPath != "old.txt" || rn.NewPath != "new.txt" {
		t.Errorf("rename paths = %q -> %q", rn.OldPath, rn.NewPath)
	}
	if rn.Additions != 1 || rn.Deletions != 1 {
		t.Errorf("rename +%d -%d", rn.Additions, rn.Deletions)
	}

	del := files[1]
	if del.Type != mailpatch.Deleted {
		t.Errorf("file1 type = %v, want deleted", del.Type)
	}
	if del.Path() != "gone.txt" {
		t.Errorf("delete path = %q", del.Path())
	}

	add := files[2]
	if add.Type != mailpatch.Added {
		t.Errorf("file2 type = %v, want added", add.Type)
	}
	if add.NewMode != "100644" {
		t.Errorf("add mode = %q", add.NewMode)
	}
}

func TestHunkParsing(t *testing.T) {
	files, err := mailpatch.ParseDiff(renameDelete)
	if err != nil {
		t.Fatal(err)
	}
	h := files[0].Hunks[0]
	if h.OldStart != 1 || h.OldLines != 3 || h.NewStart != 1 || h.NewLines != 3 {
		t.Errorf("hunk = %+v", h)
	}
	var adds, dels, ctx int
	for _, ln := range h.Lines {
		switch ln.Kind {
		case mailpatch.Add:
			adds++
		case mailpatch.Delete:
			dels++
		case mailpatch.Context:
			ctx++
		}
	}
	if adds != 1 || dels != 1 || ctx != 2 {
		t.Errorf("hunk lines: +%d -%d ctx%d", adds, dels, ctx)
	}
}

const mboxSeries = `From aaa Mon Sep 17 00:00:00 2001
From: Dev One <dev@example.com>
Date: Mon, 2 Jun 2025 10:00:00 +0000
Subject: [PATCH 0/2] add a feature

This cover letter explains the series.

---
 a.go | 1 +
 b.go | 1 +
 2 files changed, 2 insertions(+)

From bbb Mon Sep 17 00:00:00 2001
From: Dev One <dev@example.com>
Date: Mon, 2 Jun 2025 10:00:01 +0000
Subject: [PATCH 1/2] add a.go

---
 a.go | 1 +
 1 file changed, 1 insertion(+)

diff --git a/a.go b/a.go
new file mode 100644
index 0000000..111
--- /dev/null
+++ b/a.go
@@ -0,0 +1 @@
+package a
--
2.45.1

From ccc Mon Sep 17 00:00:00 2001
From: Dev One <dev@example.com>
Date: Mon, 2 Jun 2025 10:00:02 +0000
Subject: [PATCH 2/2] add b.go

---
 b.go | 1 +
 1 file changed, 1 insertion(+)

diff --git a/b.go b/b.go
new file mode 100644
index 0000000..222
--- /dev/null
+++ b/b.go
@@ -0,0 +1 @@
+package b
--
2.45.1
`

func TestParseMbox(t *testing.T) {
	patches, err := mailpatch.ParseMbox(strings.NewReader(mboxSeries))
	if err != nil {
		t.Fatal(err)
	}
	if len(patches) != 3 {
		t.Fatalf("patches = %d, want 3", len(patches))
	}
	if !patches[0].IsCoverLetter() {
		t.Error("first message should be the cover letter")
	}
	if patches[0].HasDiff() {
		t.Error("cover letter should have no diff")
	}
	if !patches[1].HasDiff() || patches[1].Files[0].Path() != "a.go" {
		t.Errorf("patch 1 wrong: %+v", patches[1].Files)
	}
}

func TestParseSeries(t *testing.T) {
	s, err := mailpatch.ParseSeries(strings.NewReader(mboxSeries))
	if err != nil {
		t.Fatal(err)
	}
	if s.Cover == nil {
		t.Fatal("Cover is nil")
	}
	if s.Len() != 2 {
		t.Fatalf("Len = %d, want 2", s.Len())
	}
	if s.Total != 2 {
		t.Errorf("Total = %d, want 2", s.Total)
	}
	if !s.Complete() {
		t.Error("series should be Complete")
	}
	if s.Patches[0].Series.Index != 1 || s.Patches[1].Series.Index != 2 {
		t.Errorf("patches out of order: %d, %d",
			s.Patches[0].Series.Index, s.Patches[1].Series.Index)
	}
}

func TestEmpty(t *testing.T) {
	if _, err := mailpatch.ParseBytes(nil); err == nil {
		t.Error("expected error on nil input")
	}
	if _, err := mailpatch.ParseMbox(strings.NewReader("   \n")); err == nil {
		t.Error("expected ErrEmpty on blank mbox")
	}
}

func TestQuotedPrintableBody(t *testing.T) {
	msg := "Subject: [PATCH] qp body\n" +
		"Content-Transfer-Encoding: quoted-printable\n" +
		"\n" +
		"line with an =3D equals sign\n"
	p, err := mailpatch.ParseBytes([]byte(msg))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(p.Body, "= equals sign") {
		t.Errorf("quoted-printable not decoded: %q", p.Body)
	}
}
