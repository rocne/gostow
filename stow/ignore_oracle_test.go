//go:build oracle

package stow

import (
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
	"testing"

	"github.com/rocne/gostow/internal/conformance"
)

// stow's t/ignore.t is 287 assertions, by far the densest file in its suite, and
// TEST-PLAN §4 ranks porting it first. Transcribing it would re-import whatever
// misreading the transcriber brought; instead this drives Stow.pm's own ignore()
// -- the exact predicate ignore.t exercises -- over the same ground, and demands
// agreement. The corpus below is drawn from ignore.t's cases and from SPEC §11.
//
// The three sources are exclusive, so each fixture chooses which exist.

type ignoreFixture struct {
	name   string
	local  *string // <stow>/pkg/.stow-local-ignore
	global *string // $HOME/.stow-global-ignore
}

func ptr(s string) *string { return &s }

func ignoreFixtures() []ignoreFixture {
	return []ignoreFixture{
		{name: "built-in defaults"},
		{name: "local: literal segment", local: ptr("bazqux\n")},
		{name: "local: segment prefix wildcard", local: ptr("baz.*\n")},
		{name: "local: segment suffix wildcard", local: ptr(".*qux\n")},
		{name: "local: path with slash", local: ptr("bar/.*x\n")},
		{name: "local: anchored path", local: ptr("^/foo/.*qux\n")},
		{name: "local: several segments", local: ptr("bar\nbaz\nqux\n")},
		{name: "local: interior path fragment", local: ptr("o/bar/b\n")},
		{name: "local: unanchored path", local: ptr("foo/bar\n")},
		{name: "local: leading slash", local: ptr("/foo/bar\n")},
		{name: "local: caret slash", local: ptr("^/foo/bar\n")},
		{name: "local: caret with quantifier", local: ptr("^/fo.+ar\n")},
		{name: "local: comments and blanks", local: ptr("\n  # a comment\n\nbar   # trailing comment\n")},
		{name: "local: escaped hash is literal", local: ptr("\\#hash\n")},
		{name: "local: duplicate patterns collapse", local: ptr("bar\nbar\nbar\n")},
		{name: "local: empty file still self-ignores", local: ptr("")},
		// A global file replaces the built-in defaults wholesale, so README
		// becomes stowable.
		{name: "global only", global: ptr("onlyglobal\n")},
		{name: "global: empty file", global: ptr("")},
		// A local file replaces the global one too.
		{name: "local beats global", local: ptr("bar\n"), global: ptr("onlyglobal\n")},
	}
}

// ignorePaths covers ignore.t's own paths plus the built-in list's edges: the
// prefix/suffix behaviour of each default pattern, path-vs-segment matching, and
// the always-appended self-ignore rule.
var ignorePaths = []string{
	"foo", "bar", "baz", "qux", "bazqux", "foo/bar", "foo/bar/baz", "foo/bar/bazqux",
	"foo/barqux", "foo/bazqux", "o/bar/b", "hash", "#hash", "onlyglobal",

	"README", "README.md", "readme", "foo/README", "foo/README.md",
	"LICENSE", "LICENSE.txt", "foo/LICENSE", "COPYING", "COPYING.md", "foo/COPYING",

	"CVS", "foo/bar/CVS", "prefix.CVS", "CVS.suffix",
	".cvsignore", "foo/bar/.cvsignore", "prefix..cvsignore", ".cvsignore.suffix",
	"#autosave#", "foo/bar/#autosave#", "prefix.#autosave#", "#autosave#.suffix",
	".#lock-file", "foo/bar/.#lock-file", ".#lock-file.suffix", "prefix..#lock-file",

	"RCS", "foo,v", "x,v", ".git", ".gitignore", ".gitmodules", "x.git",
	"_darcs", ".hg", ".svn", "backup~", "foo/backup~", "~backup",

	".stow-local-ignore", "subdir/.stow-local-ignore", "foo/.stow-local-ignore",
	".stow-global-ignore",

	"a.log", "sub/deep.log", "skip", "skip.log", "dot-foo", ".foo",
}

func TestIgnoreAgreesWithStowPm(t *testing.T) {
	// OraclePath is the single place that decides whether an oracle exists (and
	// asserts it is 2.4.1). Stow.pm ships beside that binary, so if the binary is
	// there and the module is not, the installation is broken and this must fail
	// loudly rather than skip -- a conformance test that silently skips is a
	// vacuous pass.
	perlLib := findPerlLib(t, conformance.OraclePath(t))

	for _, fx := range ignoreFixtures() {
		t.Run(fx.name, func(t *testing.T) {
			root := t.TempDir()
			for _, d := range []string{"stow/pkg", "target", "home"} {
				if err := os.MkdirAll(filepath.Join(root, d), 0o755); err != nil {
					t.Fatal(err)
				}
			}
			if fx.local != nil {
				writeFile(t, filepath.Join(root, "stow/pkg/.stow-local-ignore"), *fx.local)
			}
			if fx.global != nil {
				writeFile(t, filepath.Join(root, "home/.stow-global-ignore"), *fx.global)
			}
			home := filepath.Join(root, "home")

			want := runIgnoreOracle(t, perlLib, root, home, ignorePaths)

			t.Setenv("HOME", home)
			e, err := newEngine(Options{Dir: filepath.Join(root, "stow"), Target: filepath.Join(root, "target")})
			if err != nil {
				t.Fatalf("newEngine: %v", err)
			}

			for i, path := range ignorePaths {
				got, err := e.ignore(e.stowPath, "pkg", path)
				if err != nil {
					t.Fatalf("ignore(%q): %v", path, err)
				}
				if got != want[i] {
					t.Errorf("ignore(%q) = %v, Stow.pm says %v", path, got, want[i])
				}
			}
		})
	}
}

func runIgnoreOracle(t *testing.T, perlLib, root, home string, paths []string) []bool {
	t.Helper()

	var stdin strings.Builder
	for _, p := range paths {
		stdin.WriteString(strings.Join([]string{"../stow", "pkg", p}, "\x1f"))
		stdin.WriteString("\n")
	}

	cmd := exec.Command("perl", "testdata/ignore_oracle.pl")
	cmd.Stdin = strings.NewReader(stdin.String())
	cmd.Env = []string{
		"PATH=/usr/local/bin:/usr/bin:/bin",
		"STOW_PERL_LIB=" + perlLib,
		"ORACLE_STOW_DIR=" + filepath.Join(root, "stow"),
		"ORACLE_TARGET=" + filepath.Join(root, "target"),
		"HOME=" + home,
	}
	out, err := cmd.Output()
	if err != nil {
		t.Fatalf("running the Stow.pm ignore oracle: %v", err)
	}

	fields := strings.Fields(string(out))
	if len(fields) != len(paths) {
		t.Fatalf("oracle returned %d verdicts, want %d", len(fields), len(paths))
	}
	got := make([]bool, len(fields))
	for i, f := range fields {
		got[i] = f == "1"
	}
	return got
}

// reUseLib captures the module directory stow's build baked into its script. The
// line is absent when the install prefix is already in Perl's @INC, so its
// absence is normal, not an error.
var reUseLib = regexp.MustCompile(`(?m)^use lib "([^"]+)";`)

// findPerlLib returns the directory the ignore oracle must prepend to @INC to
// load the *pinned* Stow.pm, or "" when Perl already finds it.
//
// Guessing the layout is what made this test skip silently in CI while passing
// locally: 2.4.1 built against perl 5.40 lands in share/perl5/5.40, and a
// /usr/local install may need no `use lib` at all. So the oracle's own script is
// read for the answer, and then Perl is asked to prove it can load Stow 2.4.1.
//
// Every failure here is fatal, never a skip: an oracle binary whose module cannot
// be loaded is a broken installation, and a conformance test that skips is a
// vacuous pass.
func findPerlLib(t *testing.T, oracleBin string) string {
	t.Helper()

	script, err := os.ReadFile(oracleBin)
	if err != nil {
		t.Fatalf("reading the oracle script %s: %v", oracleBin, err)
	}
	dir := ""
	if m := reUseLib.FindSubmatch(script); m != nil {
		dir = string(m[1])
	}

	args := []string{}
	if dir != "" {
		args = append(args, "-I"+dir)
	}
	args = append(args, "-MStow", "-e", "print $Stow::VERSION")
	out, err := exec.Command("perl", args...).Output()
	if err != nil {
		t.Fatalf("perl cannot load the pinned Stow.pm (lib=%q, from %s): %v", dir, oracleBin, err)
	}
	if got := strings.TrimSpace(string(out)); got != "2.4.1" {
		t.Fatalf("perl loaded Stow %s, want 2.4.1: a mismatched module would redefine the spec", got)
	}
	return dir
}

func writeFile(t *testing.T, path, content string) {
	t.Helper()
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
}
