package cli

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/rocne/gostow/internal/conformance"
)

// --gostow-* flags answer only to their exact name. Without NoAbbrev, adding
// "gostow-fix" would make "--g" resolve to it, and `stow --g` — which real stow
// rejects — would start doing something. This is what lets gostow extend its CLI
// without denting parity for any argv that does not name an extension outright.
func TestExtensionFlagsAreNotAbbreviatable(t *testing.T) {
	root := fixture(t)
	env := map[string]string{"HOME": filepath.Join(root, "home")}

	for _, arg := range []string{"--g", "--go", "--gostow", "--gostow-f", "--gostow-"} {
		_, stderr, code := run(t, root, env, "-d", "stow", "-t", "target", arg, "pkg")
		if code != 1 {
			t.Errorf("%s: exit = %d, want 1", arg, code)
		}
		if want := "Unknown option: " + strings.TrimPrefix(arg, "--"); !strings.Contains(stderr, want) {
			t.Errorf("%s: stderr = %q, want it to contain %q", arg, stderr, want)
		}
	}
}

// The extensions are listed in --help: a flag nobody can discover is a flag
// nobody uses. Parity survives because help *prose* is not part of the contract
// — option parsing is, and that is pinned by 6307 argv vectors against real
// Getopt::Long. See SPEC §4.5.
func TestExtensionFlagsAreVisibleInHelp(t *testing.T) {
	root := fixture(t)
	stdout, _, code := run(t, root, map[string]string{"HOME": filepath.Join(root, "home")}, "--help")
	if code != 0 {
		t.Fatalf("exit = %d, want 0", code)
	}
	for _, want := range []string{"--gostow-fix", "--gostow-help"} {
		if !strings.Contains(stdout, want) {
			t.Errorf("--help should list %s", want)
		}
	}
}

// Every option the parser accepts must be documented in --help.
//
// gostow's help is no longer a transcript of stow's, so nothing external keeps
// the two in step: a new flag added to spec() and forgotten in usageText() would
// simply be undiscoverable. This is that check, and it is exactly the check GNU
// Stow lacks — `--no-folding` has been a real, working, undocumented flag there
// for years (ledger PL-16). gostow documents it.
func TestHelpDocumentsEveryOption(t *testing.T) {
	root := fixture(t)
	stdout, _, code := run(t, root, map[string]string{"HOME": filepath.Join(root, "home")}, "--help")
	if code != 0 {
		t.Fatalf("exit = %d, want 0", code)
	}

	for _, opt := range spec() {
		for _, name := range opt.Names {
			flag := "-" + name
			if len(name) > 1 {
				flag = "--" + name
			}
			if !conformance.MentionsFlag(stdout, flag) {
				t.Errorf("--help never names %s", flag)
			}
		}
	}
}

// gostow must not send its own bug reports to somebody else's mailing list. The
// copied help block used to end "Report bugs to: bug-stow@gnu.org", which is
// GNU Stow's address, and it is the reason the prose was rewritten at all.
func TestHelpPointsBugsAtGostow(t *testing.T) {
	root := fixture(t)
	stdout, _, _ := run(t, root, map[string]string{"HOME": filepath.Join(root, "home")}, "--help")

	if strings.Contains(stdout, "bug-stow@gnu.org") {
		t.Error("--help still directs gostow's bug reports to the GNU Stow mailing list")
	}
	if !strings.Contains(stdout, BugURL) {
		t.Errorf("--help should name %s", BugURL)
	}
	// stow's manual is the authority on what the shared options mean; there is
	// no second description to keep in sync, so the link must survive.
	if !strings.Contains(stdout, StowManualURL) {
		t.Errorf("--help should credit and link GNU Stow's manual at %s", StowManualURL)
	}
}

func TestGostowHelpListsTheExtensions(t *testing.T) {
	root := fixture(t)
	stdout, _, code := run(t, root, map[string]string{"HOME": filepath.Join(root, "home")}, "--gostow-help")
	if code != 0 {
		t.Fatalf("exit = %d, want 0", code)
	}
	for _, want := range []string{"--gostow-fix", "--gostow-help", "GOSTOW EXTENSIONS"} {
		if !strings.Contains(stdout, want) {
			t.Errorf("--gostow-help output should mention %q", want)
		}
	}
}

// GNU Stow prints "RMDIR /path" while every sibling prints "LINK:", "UNLINK:",
// "MKDIR:", "MV:". Reproduced by default; --gostow-fix gives it the colon.
func TestFixQuirksGivesRmdirItsColon(t *testing.T) {
	setup := func(t *testing.T) (string, map[string]string) {
		root := t.TempDir()
		for _, d := range []string{"stow/one/sub", "stow/two/sub", "target", "home"} {
			if err := os.MkdirAll(filepath.Join(root, d), 0o755); err != nil {
				t.Fatal(err)
			}
		}
		write(t, filepath.Join(root, "stow/one/sub/a"), "a")
		write(t, filepath.Join(root, "stow/two/sub/b"), "b")
		return root, map[string]string{"HOME": filepath.Join(root, "home")}
	}

	// Refolding on unstow is what produces an RMDIR.
	root, env := setup(t)
	if _, _, code := run(t, root, env, "-d", "stow", "-t", "target", "one", "two"); code != 0 {
		t.Fatalf("setup stow failed: %d", code)
	}
	_, stderr, _ := run(t, root, env, "-d", "stow", "-t", "target", "-v", "-D", "two")
	if !strings.Contains(stderr, "RMDIR sub") || strings.Contains(stderr, "RMDIR: sub") {
		t.Errorf("default must reproduce stow's colonless RMDIR, got %q", stderr)
	}

	root, env = setup(t)
	if _, _, code := run(t, root, env, "-d", "stow", "-t", "target", "one", "two"); code != 0 {
		t.Fatalf("setup stow failed: %d", code)
	}
	_, stderr, _ = run(t, root, env, "-d", "stow", "-t", "target", "-v", "--gostow-fix", "-D", "two")
	if !strings.Contains(stderr, "RMDIR: sub") {
		t.Errorf("--gostow-fix should print RMDIR with a colon, got %q", stderr)
	}
}

// `stow -- pkg` silently drops pkg: Getopt leaves it in an array stow never
// reads, and stow then dies with "No packages to stow or unstow".
func TestFixQuirksKeepsPackagesAfterDoubleDash(t *testing.T) {
	root := fixture(t)
	env := map[string]string{"HOME": filepath.Join(root, "home")}

	_, stderr, code := run(t, root, env, "-d", "stow", "-t", "target", "--", "pkg")
	if code != 1 || !strings.Contains(stderr, "No packages to stow or unstow") {
		t.Errorf("default should reproduce the drop: exit=%d stderr=%q", code, stderr)
	}

	root = fixture(t)
	env = map[string]string{"HOME": filepath.Join(root, "home")}
	_, stderr, code = run(t, root, env, "-d", "stow", "-t", "target", "--gostow-fix", "--", "pkg")
	if code != 0 {
		t.Fatalf("--gostow-fix should stow pkg: exit=%d stderr=%q", code, stderr)
	}
	if _, err := os.Lstat(filepath.Join(root, "target/f")); err != nil {
		t.Error("pkg was not stowed")
	}
}

// '#' is not a comment character to shellwords, so a .stowrc "comment" is parsed
// as ordinary tokens. A bare word becomes a package name and is discarded, which
// is why comments *appear* to work — but anything option-shaped after the '#' is
// silently honoured. Here the "commented-out" --ignore=drop really does ignore
// drop. --gostow-fix strips the comment.
func TestFixQuirksGivesStowrcRealComments(t *testing.T) {
	rc := "--dir=stow --target=target\n--ignore=keep # --ignore=drop\n"

	setup := func(t *testing.T) (string, map[string]string) {
		root := fixture(t)
		write(t, filepath.Join(root, "stow/pkg/drop"), "d")
		write(t, filepath.Join(root, ".stowrc"), rc)
		return root, map[string]string{"HOME": filepath.Join(root, "home")}
	}

	root, env := setup(t)
	if _, stderr, code := run(t, root, env, "pkg"); code != 0 {
		t.Fatalf("exit=%d stderr=%q", code, stderr)
	}
	if _, err := os.Lstat(filepath.Join(root, "target/drop")); !os.IsNotExist(err) {
		t.Error("stow honours the option after '#': drop should have been ignored, not stowed")
	}

	root, env = setup(t)
	if _, stderr, code := run(t, root, env, "--gostow-fix", "pkg"); code != 0 {
		t.Fatalf("exit=%d stderr=%q", code, stderr)
	}
	if _, err := os.Lstat(filepath.Join(root, "target/drop")); err != nil {
		t.Error("with real comments, the commented-out --ignore=drop must not apply")
	}
}
