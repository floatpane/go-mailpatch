# Security Policy

## Supported Versions

Only the latest release of go-mailpatch is supported with security updates.

## Reporting a Vulnerability

If you discover a security vulnerability in go-mailpatch, please report it responsibly. **Do not open a public issue.**

Email us at [us@floatpane.com](mailto:us@floatpane.com) with:

- A description of the vulnerability
- Steps to reproduce the issue
- The potential impact
- Any suggested fixes (optional)

We will acknowledge your report within 48 hours and aim to provide a fix or mitigation plan within 7 days, depending on severity.

## Scope

This policy covers the go-mailpatch codebase and its official releases.

go-mailpatch parses untrusted input — patch emails arrive from anyone on a mailing list — so the threats that matter are denial-of-service and parser correctness, not cryptography:

- **Resource exhaustion** — a crafted message or mbox that drives runaway memory allocation or super-linear CPU (e.g. pathological diffs, deeply nested MIME, or huge header values).
- **Catastrophic regex backtracking (ReDoS)** — an input that makes a subject, hunk, or path regular expression blow up.
- **Panics on malformed input** — any message that makes a parse function panic instead of returning an error. Callers feeding it list mail must be able to rely on errors, not recovers.
- **Mis-parsing that misleads a reviewer** — a diff whose displayed file path, change type, or add/delete counts do not match what `git am` would actually apply, in a way that could sneak a change past review.

Note the explicit non-goal (see the docs): go-mailpatch never executes git and never applies patches — it only parses. Validating or applying a patch safely is the caller's responsibility.

go-mailpatch has no third-party dependencies; only the Go standard library.

## Disclosure

We ask that you give us reasonable time to address the issue before disclosing it publicly. We are committed to crediting reporters in release notes (unless you prefer to remain anonymous).
