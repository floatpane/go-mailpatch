package mailpatch_test

import (
	"strings"
	"testing"

	mailpatch "github.com/floatpane/go-mailpatch"
)

func TestFormatBasic(t *testing.T) {
	diff := `diff --git a/foo.txt b/foo.txt
index 111..222 100644
--- a/foo.txt
+++ b/foo.txt
@@ -1,3 +1,3 @@
 line1
-old
+new
 line3
`
	raw, err := mailpatch.Format(mailpatch.FormatOptions{
		From:    "Alice <alice@example.com>",
		To:      "bob@example.com",
		Subject: "Update foo",
		Body:    "This patch updates foo.txt.\n\nThe old value was wrong.",
		Diff:    diff,
	})
	if err != nil {
		t.Fatalf("Format: %v", err)
	}
	s := string(raw)
	if !strings.Contains(s, "From: Alice <alice@example.com>") {
		t.Error("missing From header")
	}
	if !strings.Contains(s, "To: bob@example.com") {
		t.Error("missing To header")
	}
	if !strings.Contains(s, "Subject: [PATCH] Update foo") {
		t.Error("missing Subject with [PATCH] prefix; got: " + s[:200])
	}
	if !strings.Contains(s, "diff --git") {
		t.Error("missing diff content")
	}
	if !strings.Contains(s, "This patch updates foo.txt") {
		t.Error("missing commit message body")
	}
	if !strings.Contains(s, "Content-Type: text/plain") {
		t.Error("missing Content-Type header")
	}
}

func TestFormatSeries(t *testing.T) {
	diff := "diff --git a/x b/x\n--- a/x\n+++ b/x\n@@ -1 +1 @@\n-a\n+b\n"
	raw, err := mailpatch.Format(mailpatch.FormatOptions{
		From:   "Alice <alice@example.com>",
		To:     "list@example.org",
		Subject: "Fix bug",
		Diff:   diff,
		Version: 2,
		Index:  1,
		Total:  3,
	})
	if err != nil {
		t.Fatalf("Format: %v", err)
	}
	s := string(raw)
	if !strings.Contains(s, "Subject: [PATCH v2 1/3] Fix bug") {
		t.Errorf("missing series subject; got prefix section: %s", s[:200])
	}
}

func TestFormatRFC(t *testing.T) {
	diff := "diff --git a/x b/x\n--- a/x\n+++ b/x\n@@ -1 +1 @@\n-a\n+b\n"
	raw, err := mailpatch.Format(mailpatch.FormatOptions{
		From:   "Alice <alice@example.com>",
		To:     "list@example.org",
		Subject: "Experimental change",
		Diff:   diff,
		Prefix: "RFC PATCH",
	})
	if err != nil {
		t.Fatalf("Format: %v", err)
	}
	s := string(raw)
	if !strings.Contains(s, "Subject: [RFC PATCH] Experimental change") {
		t.Error("missing RFC prefix")
	}
}

func TestFormatThreading(t *testing.T) {
	diff := "diff --git a/x b/x\n--- a/x\n+++ b/x\n@@ -1 +1 @@\n-a\n+b\n"
	raw, err := mailpatch.Format(mailpatch.FormatOptions{
		From:        "Alice <alice@example.com>",
		To:          "list@example.org",
		Subject:     "Re: fix",
		Diff:        diff,
		InReplyTo:   "original-msg@example.org",
		References:  "cover@example.org original-msg@example.org",
	})
	if err != nil {
		t.Fatalf("Format: %v", err)
	}
	s := string(raw)
	if !strings.Contains(s, "In-Reply-To: <original-msg@example.org>") {
		t.Error("missing In-Reply-To header")
	}
	if !strings.Contains(s, "References: <cover@example.org> <original-msg@example.org>") {
		t.Error("missing References header")
	}
}

func TestFormatRoundTrip(t *testing.T) {
	diff := `diff --git a/greet.txt b/greet.txt
index 111..222 100644
--- a/greet.txt
+++ b/greet.txt
@@ -1,3 +1,3 @@
 hello
-world
+there
 bye
`
	raw, err := mailpatch.Format(mailpatch.FormatOptions{
		From:    "Ada <ada@example.com>",
		To:      "reviewer@example.com",
		Subject: "greet: change the world",
		Body:    "Changes greeting from world to there.",
		Diff:    diff,
	})
	if err != nil {
		t.Fatalf("Format: %v", err)
	}

	// Parse the formatted email back and verify
	p, err := mailpatch.ParseBytes(raw)
	if err != nil {
		t.Fatalf("ParseBytes: %v", err)
	}
	if p.Subject != "greet: change the world" {
		t.Errorf("Subject = %q", p.Subject)
	}
	if !p.HasDiff() {
		t.Error("HasDiff = false")
	}
	if p.Series.Index != 0 || p.Series.Total != 0 {
		t.Errorf("Series = %+v (expected 0/0 for single patch)", p.Series)
	}
}

func TestSplitBodyDiff(t *testing.T) {
	body := "This is the commit message.\n\nMore details here.\n---\n file.txt | 2 +-\n 1 file changed, 1 insertion(+), 1 deletion(-)\n\ndiff --git a/file.txt b/file.txt\n--- a/file.txt\n+++ b/file.txt\n@@ -1 +1 @@\n-old\n+new\n"
	commitMsg, diff := mailpatch.SplitBodyDiff(body)
	if !strings.Contains(commitMsg, "This is the commit message") {
		t.Errorf("commit message = %q", commitMsg)
	}
	if !strings.Contains(diff, "diff --git") {
		t.Errorf("diff = %q", diff)
	}
}
