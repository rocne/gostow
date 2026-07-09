package conformance

import (
	"path/filepath"
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
