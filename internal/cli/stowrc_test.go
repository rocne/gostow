package cli

import (
	"os"
	"path/filepath"
	"testing"
)

// The tokenizer, the parser and the expansion pipeline are tested where they
// live, in the public stowrc package. What is pinned here is the CLI's own rc
// behaviour: discovery, and the failure shape it must propagate.

// readStowrcTokens must treat a read failure as stow does: Perl's readline
// poisons the handle, close returns false, and stow dies with "Could not close
// open file". A directory is the reachable instance — open(2) succeeds and the
// first read returns EISDIR — and it is unreadable to root as well, so unlike a
// chmod-000 file this test cannot silently skip.
//
// Before the check existed, the read error sat unexamined and gostow treated the
// rc file as empty: no diagnostic, exit 0, package stowed.
func TestUnreadableStowrcIsFatal(t *testing.T) {
	dir := t.TempDir()
	t.Chdir(dir)
	t.Setenv("HOME", t.TempDir())

	if err := os.Mkdir(filepath.Join(dir, ".stowrc"), 0o755); err != nil {
		t.Fatal(err)
	}

	_, err := readStowrcTokens(false)
	if err == nil {
		t.Fatal("readStowrcTokens returned no error for a .stowrc that is a directory")
	}
	if want := "Could not close open file: .stowrc"; err.Error() != want {
		t.Errorf("readStowrcTokens error = %q, want %q", err.Error(), want)
	}
}

// PL-01: home is read first, tokens are concatenated, and the concatenation is
// parsed as one option array — so ./.stowrc wins a scalar. Concatenation is
// also why the CLI cannot parse the two files separately and merge: an option
// at the end of ~/.stowrc may legally take its value from the first token of
// ./.stowrc, and this pins that a split-and-merge refactor would break it.
func TestStowrcTokensConcatenateHomeFirst(t *testing.T) {
	home := t.TempDir()
	cwd := t.TempDir()
	t.Setenv("HOME", home)
	t.Chdir(cwd)

	if err := os.WriteFile(filepath.Join(home, ".stowrc"), []byte("--ignore=fromhome\n--target\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(cwd, ".stowrc"), []byte("valuefromcwd --ignore=fromcwd\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	tokens, err := readStowrcTokens(false)
	if err != nil {
		t.Fatal(err)
	}
	want := []string{"--ignore=fromhome", "--target", "valuefromcwd", "--ignore=fromcwd"}
	if len(tokens) != len(want) {
		t.Fatalf("tokens = %q, want %q", tokens, want)
	}
	for i := range want {
		if tokens[i] != want[i] {
			t.Fatalf("tokens = %q, want %q", tokens, want)
		}
	}
}
