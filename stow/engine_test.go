package stow

import (
	"errors"
	"os"
	"path/filepath"
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
