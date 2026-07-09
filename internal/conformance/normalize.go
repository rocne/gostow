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
// either has to fold it away first — and nothing else. Everything below the
// banner, including the synopsis, is byte-exact.
var reIdentity = regexp.MustCompile(`(?m)^(?:gostow \S+ \(GNU Stow \d+\.\d+\.\d+ compatible\)|\S+ \(GNU Stow\) version \d+\.\d+\.\d+)$`)

// NormalizeIdentity replaces the identity banner with a stable token.
func NormalizeIdentity(s string) string {
	return reIdentity.ReplaceAllString(s, "<IDENTITY>")
}

// StripExtensionLines removes every line naming a gostow extension.
//
// gostow's --help lists its own flags, which GNU Stow obviously does not. Every
// extension line contains the literal "--gostow-" and none of them adds a blank
// line, so deleting them leaves stow's block byte for byte. That is what makes
// the additive help text *checkable* rather than merely asserted: the
// differential suite strips these lines and then demands byte equality on
// everything else — the synopsis, the wording, the spacing, the trailing
// addresses.
//
// It is applied to both sides. Real stow's output never contains the token, so
// stripping is a no-op there and the comparison stays honest.
//
// This is not a licence to normalise anything else. A fixture whose *argv* names
// an extension is refused outright by AssertSameAsOracle: gostow's extended
// behaviour is never compared against an oracle that never saw the flag.
func StripExtensionLines(s string) string {
	lines := strings.Split(s, "\n")
	kept := lines[:0]
	for _, line := range lines {
		if strings.Contains(line, "--gostow-") {
			continue
		}
		kept = append(kept, line)
	}
	return strings.Join(kept, "\n")
}
