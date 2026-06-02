<div align="center">

# go-mailpatch

**Parse `git format-patch` emails in Go. Commit metadata, `[PATCH n/m]` series, unified diffs, and diffstat — zero dependencies.**

[![Go Version](https://img.shields.io/github/go-mod/go-version/floatpane/go-mailpatch)](https://golang.org)
[![Go Reference](https://pkg.go.dev/badge/github.com/floatpane/go-mailpatch.svg)](https://pkg.go.dev/github.com/floatpane/go-mailpatch)
[![GitHub release (latest by date)](https://img.shields.io/github/v/release/floatpane/go-mailpatch)](https://github.com/floatpane/go-mailpatch/releases)
[![CI](https://github.com/floatpane/go-mailpatch/actions/workflows/ci.yml/badge.svg)](https://github.com/floatpane/go-mailpatch/actions/workflows/ci.yml)
[![License: MIT](https://img.shields.io/badge/License-MIT-yellow.svg)](LICENSE)

</div>

`git format-patch` turns commits into email: the subject becomes `[PATCH 2/3] fix the thing`, the author and date become headers, the commit message becomes the body, and the diff follows a `---` separator and a diffstat. `git send-email` mails them; reviewers reply; maintainers run `git am`. `go-mailpatch` reads those messages back into structured Go — author, date, series position, commit message, and a fully parsed diff — **without shelling out to git**.

It grew out of [matcha](https://github.com/floatpane/matcha)'s developer-mail features (patch review and `git-mail`), pulled into a standalone, git-free, dependency-free library.

## Features

- **Parse one message or a whole mbox.** `Parse` for a single patch, `ParseMbox` for every message in an mbox, `ParseSeries` to group a thread into its cover letter + ordered patches.
- **Subject intelligence.** `[PATCH]`, `[PATCH 2/3]`, `[RFC PATCH v3 1/4]` — the prefix is parsed into index, total, version, and cover-letter flag, and stripped from the clean subject. Non-patch subjects (`[bug] …`) are left untouched.
- **Real diff parsing.** Per-file `FileChange` with change type (added/deleted/renamed/copied/modified), old/new paths, file modes, binary detection, and per-line hunks. Add/delete counts and a `DiffStat` come for free.
- **Standalone diff parser.** `ParseDiff` works on a bare `git diff` with no email around it.
- **MIME-aware.** Decodes quoted-printable / base64 bodies and RFC 2047 encoded-word headers; digs the `text/plain` part out of multipart mail.
- **Zero dependencies, never runs git.** Standard library only. It parses; it does not apply. What you do with the result is up to you.

## Install

```bash
go get github.com/floatpane/go-mailpatch
```

Requires Go 1.26+.

## Usage

### Parse a single patch email

```go
package main

import (
	"fmt"
	"log"
	"os"

	"github.com/floatpane/go-mailpatch"
)

func main() {
	p, err := mailpatch.Parse(os.Stdin)
	if err != nil {
		log.Fatal(err)
	}

	fmt.Printf("%s <%s>\n", p.AuthorName, p.AuthorEmail)
	fmt.Printf("Subject: %s\n", p.Subject)
	if p.Series.Total > 0 {
		fmt.Printf("Patch %d of %d (v%d)\n",
			p.Series.Index, p.Series.Total, p.Series.Version)
	}

	for _, f := range p.Files {
		fmt.Printf("  %-8s %s  +%d -%d\n",
			f.Type, f.Path(), f.Additions, f.Deletions)
	}
	fmt.Printf("%d files changed, +%d -%d\n",
		p.Stat.FilesChanged, p.Stat.Additions, p.Stat.Deletions)
}
```

```console
$ git format-patch -1 --stdout | go run .
Ada Lovelace <ada@example.com>
Subject: parser: handle empty input
Patch 2 of 3 (v1)
  modified parser.go  +3 -0
1 files changed, +3 -0
```

### Walk a patch series from an mbox

```go
f, _ := os.Open("series.mbox")
defer f.Close()

s, err := mailpatch.ParseSeries(f)
if err != nil {
	log.Fatal(err)
}

if s.Cover != nil {
	fmt.Println("cover:", s.Cover.Subject)
}
fmt.Printf("v%d, %d/%d patches present (complete=%v)\n",
	s.Version, s.Len(), s.Total, s.Complete())

for _, p := range s.Patches { // already sorted by index
	fmt.Printf("  [%d/%d] %s\n", p.Series.Index, p.Series.Total, p.Subject)
}
```

### Parse a bare diff (no email)

```go
files, _ := mailpatch.ParseDiff(diffText)
for _, f := range files {
	for _, h := range f.Hunks {
		for _, ln := range h.Lines {
			switch ln.Kind {
			case mailpatch.Add:
				fmt.Println("+", ln.Text)
			case mailpatch.Delete:
				fmt.Println("-", ln.Text)
			}
		}
	}
}
```

## What you get

| Type | Holds |
|------|-------|
| `Patch` | From / author name+email, Date, clean Subject, Message-ID / In-Reply-To / References, `SeriesInfo`, commit-message Body, raw Diff, parsed Files, DiffStat |
| `SeriesInfo` | Index, Total, Version, Prefix, IsCover |
| `FileChange` | OldPath / NewPath, Type, IsBinary, Old/NewMode, Additions / Deletions, Hunks |
| `Hunk` | OldStart/OldLines, NewStart/NewLines, Section, Lines |
| `Line` | Kind (Context / Add / Delete), Text |
| `DiffStat` | FilesChanged, Additions, Deletions |
| `Series` | Cover, ordered Patches, Version, Total |

## What this is not

- **Not a patch applier.** It never runs `git am` and never touches your working tree. Parse here, apply (and validate) yourself.
- **Not a diff renderer.** It gives you the structure; coloring/printing is the caller's job.
- **Not a full RFC 5322 MTA.** It handles the headers and encodings that format-patch mail uses in practice, not every corner of email.

## Documentation

Full API reference: [pkg.go.dev/github.com/floatpane/go-mailpatch](https://pkg.go.dev/github.com/floatpane/go-mailpatch)

Guides and diagrams: see [`docs/`](docs/).

## Sister projects

| Project | Role |
|---------|------|
| [floatpane/matcha](https://github.com/floatpane/matcha) | Reference consumer — uses this library for patch review and `git-mail`. |
| [floatpane/go-secretbox](https://github.com/floatpane/go-secretbox) | Sibling extraction — password-based encryption for data at rest. |

## Contributing

PRs welcome. See [CONTRIBUTING.md](CONTRIBUTING.md).

## Security

It parses untrusted list mail. Report vulnerabilities privately via [SECURITY.md](SECURITY.md).

## License

MIT. See [LICENSE](LICENSE).
