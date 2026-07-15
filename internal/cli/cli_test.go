package cli

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func run(t *testing.T, cwd string, env map[string]string, args ...string) (stdout, stderr string, code int) {
	t.Helper()
	for k, v := range env {
		t.Setenv(k, v)
	}
	wd, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	if err := os.Chdir(cwd); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.Chdir(wd) })

	var out, errb bytes.Buffer
	code = Run(append([]string{"stow"}, args...), "0.1.0", &out, &errb)
	return out.String(), errb.String(), code
}

func fixture(t *testing.T) string {
	t.Helper()
	root := t.TempDir()
	for _, d := range []string{"stow/pkg", "target", "home"} {
		if err := os.MkdirAll(filepath.Join(root, d), 0o755); err != nil {
			t.Fatal(err)
		}
	}
	if err := os.WriteFile(filepath.Join(root, "stow/pkg/f"), []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	return root
}

// Ledger PL-18. Stow.pm interpolates $ENV{HOME} into a regex unescaped, so a
// home directory containing a regex metacharacter kills real stow before it does
// any work — at every verbosity, because the substitution precedes the debug()
// guard. gostow does not build that regex at all.
func TestHomeWithRegexMetacharactersIsHarmless(t *testing.T) {
	root := fixture(t)
	home := filepath.Join(root, "ho(me")
	if err := os.MkdirAll(home, 0o755); err != nil {
		t.Fatal(err)
	}

	_, stderr, code := run(t, root, map[string]string{"HOME": home}, "-d", "stow", "-t", "target", "pkg")
	if code != 0 {
		t.Fatalf("exit = %d, want 0 (real stow dies here); stderr: %s", code, stderr)
	}
	if _, err := os.Lstat(filepath.Join(root, "target/f")); err != nil {
		t.Error("the package was not stowed")
	}
}

// A usage error puts the diagnostic on stderr and the whole usage block on
// stdout, and exits 1. Nothing but usage and version ever reaches stdout.
func TestUsageErrorStreamsAndExitCode(t *testing.T) {
	root := fixture(t)
	stdout, stderr, code := run(t, root, map[string]string{"HOME": filepath.Join(root, "home")},
		"-d", "stow", "-t", "target")

	if code != 1 {
		t.Errorf("exit = %d, want 1", code)
	}
	if want := "stow: No packages to stow or unstow\n\n"; stderr != want {
		t.Errorf("stderr = %q, want %q", stderr, want)
	}
	if !strings.HasPrefix(stdout, "gostow 0.1.0 (GNU Stow 2.4.1 compatible)\n") {
		t.Errorf("stdout should open with the identity line, got %q", firstLine(stdout))
	}
	if !strings.Contains(stdout, "    stow [OPTION ...]") {
		t.Error("the synopsis should follow basename($0), which is 'stow' here")
	}
}

// A bare die() reaches stderr unadorned; error() is prefixed. Both exit 2.
func TestUndefinedRcVariableDiesWithoutPrefix(t *testing.T) {
	root := fixture(t)
	if err := os.WriteFile(filepath.Join(root, ".stowrc"), []byte("--dir=stow\n--target=$NOPE_UNDEFINED\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	_, stderr, code := run(t, root, map[string]string{"HOME": filepath.Join(root, "home")}, "pkg")

	if code != 2 {
		t.Errorf("exit = %d, want 2", code)
	}
	want := "--target option references undefined environment variable $NOPE_UNDEFINED; aborting!\n"
	if stderr != want {
		t.Errorf("stderr = %q, want %q", stderr, want)
	}
}

func TestSlashInPackageNameIsPrefixedFatal(t *testing.T) {
	root := fixture(t)
	_, stderr, code := run(t, root, map[string]string{"HOME": filepath.Join(root, "home")},
		"-d", "stow", "-t", "target", "pkg/sub")

	if code != 2 {
		t.Errorf("exit = %d, want 2", code)
	}
	if want := "stow: ERROR: Slashes are not permitted in package names\n"; stderr != want {
		t.Errorf("stderr = %q, want %q", stderr, want)
	}
}

func firstLine(s string) string {
	if i := strings.IndexByte(s, '\n'); i >= 0 {
		return s[:i]
	}
	return s
}

func write(t *testing.T, path, content string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
}
