package cli

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/rocne/gostow/internal/ui"
)

// through renders s the way a TTY would see it, then strips the escapes back
// out. Anything but the identity means colour has re-laid out the text.
func through(t *testing.T, s string) string {
	t.Helper()
	var buf bytes.Buffer
	w := ui.NewWriter(&buf, true)
	if _, err := w.Write([]byte(s)); err != nil {
		t.Fatal(err)
	}
	if err := w.Flush(); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(buf.String(), "\x1b[") {
		t.Fatal("nothing was painted; the round trip below would be vacuous")
	}
	return ui.StripANSI(buf.String())
}

// TestColouredHelpIsARendering is the constraint SPEC §8.4 places on colour: the
// help block is a fixed string dictated by parity, so colour may only tint it in
// place. If a rule ever inserted, dropped or rewrapped a byte, `gostow --help`
// on a TTY would stop being `stow --help`.
func TestColouredHelpIsARendering(t *testing.T) {
	for _, tc := range []struct {
		name string
		text string
	}{
		{"help", usageText("stow", "0.1.0")},
		{"extension help", extensionHelp("0.1.0")},
		{"identity line", IdentityLine("0.1.0") + "\n"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if got := through(t, tc.text); got != tc.text {
				t.Errorf("colour altered %s:\n got %q\nwant %q", tc.name, got, tc.text)
			}
		})
	}
}

// TestNoColourOffATTY is the other half: every byte gostow writes to a pipe is
// stow's. The whole test suite and the differential harness capture output into
// buffers, so this is what makes their byte comparisons meaningful -- and it
// fails loudly if someone ever writes an escape at a call site instead of
// through internal/ui.
func TestNoColourOffATTY(t *testing.T) {
	for _, tc := range []struct {
		name string
		args []string
	}{
		{"help", []string{"--help"}},
		{"version", []string{"--version"}},
		{"extension help", []string{"--gostow-help"}},
		{"usage error", []string{"--nosuchflag"}},
		{"fatal error", []string{"-d", "stow", "-t", "target", "nosuchpkg"}},
		{"verbose stow", []string{"-v", "-v", "-d", "stow", "-t", "target", "pkg"}},
		{"conflict", []string{"-d", "stow", "-t", "target", "pkg", "pkg2"}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			root := fixture(t)
			// A second package colliding on the same file name, so the conflict
			// case reaches "WARNING!", "  * ..." and "All operations aborted."
			mustWrite(t, filepath.Join(root, "stow/pkg2/f"), "y")

			env := map[string]string{"HOME": filepath.Join(root, "home")}
			stdout, stderr, _ := run(t, root, env, tc.args...)
			for stream, s := range map[string]string{"stdout": stdout, "stderr": stderr} {
				if strings.Contains(s, "\x1b") {
					t.Errorf("%s carried an escape sequence off a TTY: %q", stream, s)
				}
			}
		})
	}
}

func mustWrite(t *testing.T, path, content string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
}
