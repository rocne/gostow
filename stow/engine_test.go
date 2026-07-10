package stow

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// Ported from stow 2.4.1's t/dotfiles.t plus the edge cases in SPEC §6, all of
// which were probed against the real binary.
func TestAdjustDotfile(t *testing.T) {
	tests := []struct{ in, want string }{
		{"dot-bashrc", ".bashrc"},
		{"dot-config", ".config"},
		{"dot--dash", ".-dash"},        // "-" satisfies [^.]
		{"dot-", "dot-"},               // nothing follows the prefix
		{"dot-.hidden", "dot-.hidden"}, // the next character is a dot
		{"notdot-x", "notdot-x"},       // the prefix is anchored
		{"plain", "plain"},
	}
	for _, tt := range tests {
		if got := adjustDotfile(tt.in); got != tt.want {
			t.Errorf("adjustDotfile(%q) = %q, want %q", tt.in, got, tt.want)
		}
	}
}

func TestUnadjustDotfile(t *testing.T) {
	tests := []struct{ in, want string }{
		{".bashrc", "dot-bashrc"},
		{".", "."},   // exempt
		{"..", ".."}, // exempt
		{"plain", "plain"},
	}
	for _, tt := range tests {
		if got := unadjustDotfile(tt.in); got != tt.want {
			t.Errorf("unadjustDotfile(%q) = %q, want %q", tt.in, got, tt.want)
		}
	}
}

// sandbox builds <root>/{stow,target,home} and returns the root.
func sandbox(t *testing.T) string {
	t.Helper()
	root := t.TempDir()
	for _, d := range []string{"stow", "target", "home"} {
		if err := os.MkdirAll(filepath.Join(root, d), 0o755); err != nil {
			t.Fatal(err)
		}
	}
	t.Setenv("HOME", filepath.Join(root, "home"))
	return root
}

func opts(root string) Options {
	return Options{Dir: filepath.Join(root, "stow"), Target: filepath.Join(root, "target"), Fold: true}
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

func TestStowCreatesRelativeLink(t *testing.T) {
	root := sandbox(t)
	write(t, filepath.Join(root, "stow/pkg/f"), "x")

	if _, err := Stow(opts(root), "pkg"); err != nil {
		t.Fatalf("Stow: %v", err)
	}
	dest, err := os.Readlink(filepath.Join(root, "target/f"))
	if err != nil {
		t.Fatalf("readlink: %v", err)
	}
	// The link must be relative. An absolute link would break the moment the
	// tree is moved, and it is not what stow writes.
	if want := "../stow/pkg/f"; dest != want {
		t.Errorf("link destination = %q, want %q", dest, want)
	}
}

func TestSimulateWritesNothing(t *testing.T) {
	root := sandbox(t)
	write(t, filepath.Join(root, "stow/pkg/f"), "x")

	o := opts(root)
	o.Simulate = true
	res, err := Stow(o, "pkg")
	if err != nil {
		t.Fatalf("Stow: %v", err)
	}
	if len(res.Tasks) != 1 {
		t.Errorf("planned %d tasks, want 1", len(res.Tasks))
	}
	if _, err := os.Lstat(filepath.Join(root, "target/f")); !os.IsNotExist(err) {
		t.Error("simulate mode touched the filesystem")
	}
}

// A conflict aborts everything, including work planned for other packages. This
// is stow's all-or-nothing semantics, and the reason Apply — not per-package
// Stow — is the real interface.
func TestConflictAbortsEveryPackage(t *testing.T) {
	root := sandbox(t)
	write(t, filepath.Join(root, "stow/good/g"), "g")
	write(t, filepath.Join(root, "stow/bad/b"), "b")
	write(t, filepath.Join(root, "target/b"), "pre-existing")

	_, err := Stow(opts(root), "good", "bad")

	var ce *ConflictError
	if !errors.As(err, &ce) {
		t.Fatalf("Stow: err = %v, want *ConflictError", err)
	}
	if _, err := os.Lstat(filepath.Join(root, "target/g")); !os.IsNotExist(err) {
		t.Error("a conflict in one package did not prevent another package being stowed")
	}
}

// Ledger PL-09. Real stow aborts the whole unstow with "Could not read link"
// because Perl reads the destination "0" as false. gostow uses `defined`.
func TestUnstowToleratesLinkPointingAtZero(t *testing.T) {
	root := sandbox(t)
	write(t, filepath.Join(root, "stow/pkg/f"), "x")
	if _, err := Stow(opts(root), "pkg"); err != nil {
		t.Fatalf("Stow: %v", err)
	}
	if err := os.Symlink("0", filepath.Join(root, "target/zerolink")); err != nil {
		t.Fatal(err)
	}

	if _, err := Unstow(opts(root), "pkg"); err != nil {
		t.Fatalf("Unstow: %v (real stow fails here; PL-09 rules that a bug)", err)
	}
	if _, err := os.Lstat(filepath.Join(root, "target/f")); !os.IsNotExist(err) {
		t.Error("the package link survived the unstow")
	}
	if _, err := os.Lstat(filepath.Join(root, "target/zerolink")); err != nil {
		t.Error("the unrelated link pointing at \"0\" was removed")
	}
}

// Ledger PL-10. Real stow silently disables *all* ignoring — stowing README.md
// and the ignore file itself — and exits 0. gostow fails loudly instead.
//
// Two ways to make the file exist but not read. `chmod 000` is the natural one
// and is what a user hits, but it does nothing to root, so under `sudo go test`
// that case can only skip. A directory named `.stow-local-ignore` is unreadable
// to *everyone*, root included, and reaches exactly the same code path — so the
// ruling is always asserted, whoever runs the suite. A skipped case is a case
// nobody tested.
func TestUnreadableIgnoreFileIsFatal(t *testing.T) {
	makeUnreadable := map[string]func(t *testing.T, path string){
		"a directory in its place": func(t *testing.T, path string) {
			if err := os.Mkdir(path, 0o755); err != nil {
				t.Fatal(err)
			}
		},
		"chmod 000": func(t *testing.T, path string) {
			if os.Geteuid() == 0 {
				t.Skip("running as root: chmod 000 does not prevent reads (the directory case above still asserts the ruling)")
			}
			write(t, path, "something\n")
			if err := os.Chmod(path, 0o000); err != nil {
				t.Fatal(err)
			}
		},
	}

	for name, breakIt := range makeUnreadable {
		t.Run(name, func(t *testing.T) {
			root := sandbox(t)
			write(t, filepath.Join(root, "stow/pkg/f"), "x")
			write(t, filepath.Join(root, "stow/pkg/README.md"), "readme")
			breakIt(t, filepath.Join(root, "stow/pkg/.stow-local-ignore"))

			_, err := Stow(opts(root), "pkg")

			var fe *FatalError
			if !errors.As(err, &fe) {
				t.Fatalf("Stow: err = %v, want *FatalError", err)
			}
			if _, err := os.Lstat(filepath.Join(root, "target/README.md")); !os.IsNotExist(err) {
				t.Error("README.md was stowed: the built-in ignore list was silently disabled")
			}
		})
	}
}

// The three ignore sources are exclusive. A ~/.stow-global-ignore discards the
// built-in defaults entirely, so README.md becomes stowable.
func TestGlobalIgnoreFileDiscardsBuiltinDefaults(t *testing.T) {
	root := sandbox(t)
	write(t, filepath.Join(root, "stow/pkg/README.md"), "readme")
	write(t, filepath.Join(root, "home/.stow-global-ignore"), "nothing-matches\n")

	if _, err := Stow(opts(root), "pkg"); err != nil {
		t.Fatalf("Stow: %v", err)
	}
	if _, err := os.Lstat(filepath.Join(root, "target/README.md")); err != nil {
		t.Error("README.md was ignored even though a global ignore file replaced the defaults")
	}
}

func TestBuiltinIgnoreListSkipsReadme(t *testing.T) {
	root := sandbox(t)
	write(t, filepath.Join(root, "stow/pkg/README.md"), "readme")
	write(t, filepath.Join(root, "stow/pkg/f"), "f")

	if _, err := Stow(opts(root), "pkg"); err != nil {
		t.Fatalf("Stow: %v", err)
	}
	if _, err := os.Lstat(filepath.Join(root, "target/README.md")); !os.IsNotExist(err) {
		t.Error("README.md should be ignored by the built-in defaults")
	}
	if _, err := os.Lstat(filepath.Join(root, "target/f")); err != nil {
		t.Error("f should have been stowed")
	}
}

func TestMissingPackageIsFatal(t *testing.T) {
	root := sandbox(t)
	_, err := Stow(opts(root), "nope")
	var fe *FatalError
	if !errors.As(err, &fe) {
		t.Fatalf("Stow: err = %v, want *FatalError", err)
	}
	if want := "The stow directory ../stow does not contain package nope"; fe.Msg != want {
		t.Errorf("message = %q, want %q", fe.Msg, want)
	}
}

// The one place GNU Stow's mistake is shaped into this code. stow asks
// should_skip_target about the *package* subdir when stowing, but the *target*
// subdir when unstowing. Under --dotfiles those are different names, so stowing
// walks past a .stow guard that unstowing respects — creating files inside a
// protected directory that the matching unstow then refuses to remove.
//
// Reproduced by default. FixQuirks asks the right question.
func TestDotfilesProtectionBypass(t *testing.T) {
	build := func(t *testing.T) string {
		root := sandbox(t)
		write(t, filepath.Join(root, "stow/pkg/dot-foo/bar"), "x")
		write(t, filepath.Join(root, "target/.foo/.stow"), "")
		return root
	}

	root := build(t)
	o := opts(root)
	o.Dotfiles = true
	if _, err := Stow(o, "pkg"); err != nil {
		t.Fatalf("Stow: %v", err)
	}
	if _, err := os.Lstat(filepath.Join(root, "target/.foo/bar")); err != nil {
		t.Error("parity requires reproducing the bypass: stow does create .foo/bar here")
	}

	root = build(t)
	o = opts(root)
	o.Dotfiles = true
	o.FixQuirks = true
	if _, err := Stow(o, "pkg"); err != nil {
		t.Fatalf("Stow: %v", err)
	}
	if _, err := os.Lstat(filepath.Join(root, "target/.foo/bar")); !os.IsNotExist(err) {
		t.Error("FixQuirks must honour the .stow guard when stowing, as unstowing already does")
	}
}

// FixQuirks changes nothing about an ordinary run: it is a lever for a small,
// enumerated set of defects, not a different engine.
func TestFixQuirksDoesNotDisturbOrdinaryStowing(t *testing.T) {
	for _, fix := range []bool{false, true} {
		root := sandbox(t)
		write(t, filepath.Join(root, "stow/pkg/sub/a"), "a")
		o := opts(root)
		o.FixQuirks = fix
		if _, err := Stow(o, "pkg"); err != nil {
			t.Fatalf("FixQuirks=%v: %v", fix, err)
		}
		dest, err := os.Readlink(filepath.Join(root, "target/sub"))
		if err != nil {
			t.Fatalf("FixQuirks=%v: readlink: %v", fix, err)
		}
		if want := "../stow/pkg/sub"; dest != want {
			t.Errorf("FixQuirks=%v: link = %q, want %q", fix, dest, want)
		}
	}
}

// Apply used to have no default arm: an unknown Action planned nothing and
// returned success. For a semver-bound library that dstow drives
// programmatically, a silent no-op on a caller's bug is the worst available
// answer — the engine already treats an impossible TaskAction as fatal, and an
// impossible Action is the same kind of mistake.
func TestApplyRejectsAnUnknownAction(t *testing.T) {
	dir, target := t.TempDir(), t.TempDir()
	if err := os.MkdirAll(filepath.Join(dir, "pkg"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "pkg", "f"), []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}

	_, err := Apply(Options{Dir: dir, Target: target, Fold: true},
		Request{Action: Action(7), Packages: []string{"pkg"}})

	var fe *FatalError
	if !errors.As(err, &fe) {
		t.Fatalf("Apply with a bogus action returned %v, want a FatalError", err)
	}
	if _, err := os.Lstat(filepath.Join(target, "f")); err == nil {
		t.Error("Apply with a bogus action wrote to the target")
	}
}

// Gerund is the word stow interpolates into both the level-2 CONFLICT line and
// the conflict banner. It is a table rather than String()+"ing" precisely so a
// rename of the Stringer cannot move those bytes; assert the table, so a rename
// of Gerund's arms cannot either.
func TestGerund(t *testing.T) {
	for action, want := range map[Action]string{
		ActionStow:   "stowing",
		ActionUnstow: "unstowing",
		ActionRestow: "restowing",
	} {
		if got := Gerund(action); got != want {
			t.Errorf("Gerund(%v) = %q, want %q", action, got, want)
		}
	}
}

// Action.String has no caller in gostow. It exists so %v prints something sane in
// a library consumer's logs, and it is documented as free to be renamed — which
// is only safe while nothing parity-pinned reads it. Pin the text so a rename is
// a deliberate act rather than a silent change to somebody else's log lines.
func TestActionString(t *testing.T) {
	for action, want := range map[Action]string{
		ActionStow:   "stow",
		ActionUnstow: "unstow",
		ActionRestow: "restow",
	} {
		if got := action.String(); got != want {
			t.Errorf("Action(%d).String() = %q, want %q", int(action), got, want)
		}
	}
}

// doMv, isRealDir and Restow all read 0% in a hermetic coverage run, and the
// reason is not that nothing exercises them: the differential and golden layers
// drive gostow as a *subprocess*, and `go test -coverprofile` cannot see a
// binary it did not link. The same blind spot as the test cache and the oracle.
//
// So exercise them in process. --adopt is the only path that ever files a move
// task, and a move is the only operation here that destroys information.
func TestAdoptMovesAConflictingFileIntoThePackage(t *testing.T) {
	dir, target := t.TempDir(), t.TempDir()
	write(t, filepath.Join(dir, "pkg", "f"), "package version")
	write(t, filepath.Join(target, "f"), "target version")

	if _, err := Stow(Options{Dir: dir, Target: target, Fold: true, Adopt: true}, "pkg"); err != nil {
		t.Fatalf("Stow --adopt: %v", err)
	}

	// The target's content wins: stow moves the target file over the package's,
	// then links to it. Adopting is how you take an existing dotfile into a
	// package without retyping it, so the *target* is the thing preserved.
	got, err := os.ReadFile(filepath.Join(dir, "pkg", "f"))
	if err != nil {
		t.Fatal(err)
	}
	if string(got) != "target version" {
		t.Errorf("package file = %q, want the adopted target's content", got)
	}

	fi, err := os.Lstat(filepath.Join(target, "f"))
	if err != nil {
		t.Fatal(err)
	}
	if fi.Mode()&os.ModeSymlink == 0 {
		t.Error("target/f is not a symlink after --adopt")
	}
	dest, err := os.Readlink(filepath.Join(target, "f"))
	if err != nil {
		t.Fatal(err)
	}
	if want := filepath.Join("..", filepath.Base(dir), "pkg", "f"); dest != want {
		t.Errorf("target/f -> %q, want %q", dest, want)
	}
}

// isRealDir distinguishes a directory from a symlink to one, which only matters
// when folding is off and the walk must descend rather than link.
func TestNoFoldingCreatesDirectoriesRatherThanFoldingThem(t *testing.T) {
	dir, target := t.TempDir(), t.TempDir()
	write(t, filepath.Join(dir, "pkg", "sub", "a"), "a")

	if _, err := Stow(Options{Dir: dir, Target: target, Fold: false}, "pkg"); err != nil {
		t.Fatalf("Stow --no-folding: %v", err)
	}

	fi, err := os.Lstat(filepath.Join(target, "sub"))
	if err != nil {
		t.Fatal(err)
	}
	if fi.Mode()&os.ModeSymlink != 0 {
		t.Fatal("target/sub is a symlink; --no-folding must create a real directory")
	}
	if !fi.IsDir() {
		t.Fatal("target/sub is not a directory")
	}
	if _, err := os.Readlink(filepath.Join(target, "sub", "a")); err != nil {
		t.Errorf("target/sub/a is not a symlink: %v", err)
	}
}

// Restow is three lines of sugar over Apply, and a library consumer's first call
// should not be its first execution ever.
func TestRestowUnstowsThenStows(t *testing.T) {
	dir, target := t.TempDir(), t.TempDir()
	write(t, filepath.Join(dir, "pkg", "old"), "old")

	opts := Options{Dir: dir, Target: target, Fold: true}
	if _, err := Stow(opts, "pkg"); err != nil {
		t.Fatalf("Stow: %v", err)
	}

	// Rename the package file. A restow must remove the now-dangling link and
	// create the new one; a bare stow would leave the stale link behind.
	if err := os.Rename(filepath.Join(dir, "pkg", "old"), filepath.Join(dir, "pkg", "new")); err != nil {
		t.Fatal(err)
	}
	if _, err := Restow(opts, "pkg"); err != nil {
		t.Fatalf("Restow: %v", err)
	}

	if _, err := os.Lstat(filepath.Join(target, "old")); !os.IsNotExist(err) {
		t.Errorf("target/old survived the restow (err = %v)", err)
	}
	if _, err := os.Readlink(filepath.Join(target, "new")); err != nil {
		t.Errorf("target/new was not created: %v", err)
	}
}

// Unstow is the same shape of sugar, and the only one whose failure would be
// silent: an Unstow that planned a stow would look like a no-op on an empty tree.
func TestUnstowRemovesWhatStowCreated(t *testing.T) {
	dir, target := t.TempDir(), t.TempDir()
	write(t, filepath.Join(dir, "pkg", "f"), "x")

	opts := Options{Dir: dir, Target: target, Fold: true}
	if _, err := Stow(opts, "pkg"); err != nil {
		t.Fatalf("Stow: %v", err)
	}
	if _, err := os.Readlink(filepath.Join(target, "f")); err != nil {
		t.Fatalf("Stow created no link: %v", err)
	}
	if _, err := Unstow(opts, "pkg"); err != nil {
		t.Fatalf("Unstow: %v", err)
	}
	if _, err := os.Lstat(filepath.Join(target, "f")); !os.IsNotExist(err) {
		t.Errorf("target/f survived the unstow (err = %v)", err)
	}
}

// canon_path chdirs into the path and dies when it cannot. Reaching that in
// process without root-only tricks: give it a path whose parent is a regular
// file. ENOTDIR is unsearchable to everyone, including root, so this case can
// never quietly skip the way a chmod-000 one would.
//
// The old canonPath swallowed every EvalSymlinks failure and returned the
// absolute path, on the theory that a not-yet-existing path canonicalises to its
// absolute form. It made `stow -n` report success on a target stow refuses.
func TestCanonPathIsFatalWhenItCannotEnterThePath(t *testing.T) {
	tmp := t.TempDir()
	dir := t.TempDir()
	write(t, filepath.Join(dir, "pkg", "f"), "x")
	write(t, filepath.Join(tmp, "notadir"), "I am a file")

	for _, target := range []string{
		filepath.Join(tmp, "notadir", "under-a-file"),
		filepath.Join(tmp, "does-not-exist"),
	} {
		_, err := Stow(Options{Dir: dir, Target: target, Fold: true, Simulate: true}, "pkg")

		var fe *FatalError
		if !errors.As(err, &fe) {
			t.Errorf("Stow into %q returned %v, want a FatalError", target, err)
			continue
		}
		if want := "canon_path: cannot chdir to " + target; !strings.HasPrefix(fe.Msg, want) {
			t.Errorf("Stow into %q: message %q, want it to start %q", target, fe.Msg, want)
		}
	}
}

// Perl imposes no line-length limit on an ignore file; a bufio.Scanner errors
// past 64 KiB. gostow must read the long pattern, not reject a file real stow
// reads without complaint. Audit item U2.
func TestIgnoreFileLineLongerThanAScannerBuffer(t *testing.T) {
	dir, target := t.TempDir(), t.TempDir()
	pkg := filepath.Join(dir, "pkg")
	write(t, filepath.Join(pkg, "keep"), "k")
	write(t, filepath.Join(pkg, "drop"), "d")

	// One pattern, far past the Scanner's limit, that still matches "drop".
	long := "(" + strings.Repeat("nomatch|", 12000) + "drop)"
	if len(long) < 64*1024 {
		t.Fatalf("pattern is only %d bytes; this test would not reach the limit", len(long))
	}
	write(t, filepath.Join(pkg, ".stow-local-ignore"), long+"\n")

	if _, err := Stow(Options{Dir: dir, Target: target, Fold: false}, "pkg"); err != nil {
		t.Fatalf("Stow with a long ignore line: %v", err)
	}
	if _, err := os.Lstat(filepath.Join(target, "keep")); err != nil {
		t.Errorf("keep was not stowed: %v", err)
	}
	if _, err := os.Lstat(filepath.Join(target, "drop")); !os.IsNotExist(err) {
		t.Errorf("drop was stowed despite the long ignore pattern (err = %v)", err)
	}
}
