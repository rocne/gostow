package conformance

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// The sandbox root reaches the streams in two forms, and one *contains* the other.
//
// On darwin, t.TempDir() returns /var/folders/..., a symlink to
// /private/var/folders/... — the resolved path is the unresolved one with a
// prefix glued on. Stow::Util::canon_path chdirs and calls getcwd, so real stow
// prints the resolved form, and so does gostow. Replacing the unresolved root
// first therefore rewrites the *tail* of the resolved one and leaves
// "/private$SANDBOX" behind. That is what made the macOS job fail against goldens
// recorded on Linux, where the two paths are identical.
//
// The containment is the whole bug, so the fixture must reproduce it — a symlink
// whose target merely lives elsewhere would pass under either ordering. Built by
// hand here so every platform runs it, not only the CI runner nobody develops on.
func TestNormalizeReplacesTheLongerRootFirst(t *testing.T) {
	base := t.TempDir()

	link := filepath.Join(base, "var", "sandbox")
	// "/private" + link, exactly as darwin does it: the real path ends with the
	// link's own absolute path.
	real := filepath.Join(base, "private") + link
	if !strings.HasSuffix(real, link) {
		t.Fatalf("fixture is wrong: %q does not end with %q", real, link)
	}

	if err := os.MkdirAll(real, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Dir(link), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(real, link); err != nil {
		t.Fatal(err)
	}

	// The engine resolves symlinks, so it prints `real`; the harness knows `link`.
	line := "stow dir is " + real + "/stow"
	if got, want := Normalize(line, link), "stow dir is $SANDBOX/stow"; got != want {
		t.Errorf("Normalize(%q, %q)\n = %q\nwant %q", line, link, got, want)
	}

	// The unresolved form must still normalise, and an unrelated path must not.
	if got, want := Normalize("cwd "+link, link), "cwd $SANDBOX"; got != want {
		t.Errorf("Normalize of the unresolved root = %q, want %q", got, want)
	}
	if got := Normalize("/elsewhere/x", link); got != "/elsewhere/x" {
		t.Errorf("Normalize rewrote an unrelated path: %q", got)
	}
}
