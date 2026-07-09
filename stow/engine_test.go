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
func TestUnreadableIgnoreFileIsFatal(t *testing.T) {
	if os.Geteuid() == 0 {
		t.Skip("running as root: chmod 000 does not prevent reads")
	}
	root := sandbox(t)
	write(t, filepath.Join(root, "stow/pkg/f"), "x")
	write(t, filepath.Join(root, "stow/pkg/README.md"), "readme")
	ignoreFile := filepath.Join(root, "stow/pkg/.stow-local-ignore")
	write(t, ignoreFile, "something\n")
	if err := os.Chmod(ignoreFile, 0o000); err != nil {
		t.Fatal(err)
	}

	_, err := Stow(opts(root), "pkg")

	var fe *FatalError
	if !errors.As(err, &fe) {
		t.Fatalf("Stow: err = %v, want *FatalError", err)
	}
	if _, err := os.Lstat(filepath.Join(root, "target/README.md")); !os.IsNotExist(err) {
		t.Error("README.md was stowed: the built-in ignore list was silently disabled")
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
