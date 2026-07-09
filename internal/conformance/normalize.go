package conformance

import (
	"path/filepath"
	"regexp"
	"strings"
)

// Normalize replaces the absolute sandbox root with the token "$SANDBOX" so
// output that leaks paths (stow prints "(cwd=...)" at verbosity ≥3) can be
// compared across two different temp roots. The EvalSymlinks form is replaced
// too: on systems where /tmp is a symlink, Perl's getcwd() reports the resolved
// path while Go's t.TempDir() does not. Nothing else is touched — no whitespace,
// line-order, or case normalisation, so "RMDIR bin" stays unequal to
// "RMDIR: bin" (ledger PL-05).
func Normalize(s string, root string) string {
	s = strings.ReplaceAll(s, root, "$SANDBOX")
	if resolved, err := filepath.EvalSymlinks(root); err == nil && resolved != root {
		s = strings.ReplaceAll(s, resolved, "$SANDBOX")
	}
	return s
}

// reIdentity matches the one line gostow deliberately does not reproduce: stow's
// "<prog> (GNU Stow) version 2.4.1" versus gostow's
// "gostow <ver> (GNU Stow 2.4.1 compatible)". Ledger PL-12 rules that divergence.
// It opens both --version and the --help block, so a byte-for-byte comparison of
// either has to fold it away first — and nothing else.
var reIdentity = regexp.MustCompile(`(?m)^(?:gostow \S+ \(GNU Stow \d+\.\d+\.\d+ compatible\)|\S+ \(GNU Stow\) version \d+\.\d+\.\d+)$`)

// NormalizeIdentity replaces the identity banner with a stable token.
func NormalizeIdentity(s string) string {
	return reIdentity.ReplaceAllString(s, "<IDENTITY>")
}

// There was once a StripExtensionLines here, deleting gostow's "--gostow-" lines
// from --help so the rest could be compared to stow's block byte for byte. It is
// gone with the transcript it served: gostow's help is now its own prose (SPEC
// §4.5), so there is no block to rejoin. What replaced it is not a looser check
// but a different one — Case.UsageOnStdout requires each binary's usage error to
// print exactly that binary's own --help, and the oracle suite requires every
// option stow documents to be documented by gostow too.
//
// The argv guard in AssertSameAsOracle stays. Filtering *output* was defensible;
// filtering an *argument* would compare two different commands.
