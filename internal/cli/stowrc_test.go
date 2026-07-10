package cli

import (
	"os"
	"os/user"
	"path/filepath"
	"strings"
	"testing"
)

// The ruling of ledger PL-21, pinned.
//
// Real stow expands `~nosuchuser` to the empty string, because Perl's
// (getpwnam($1))[7] is undef and interpolates as "". `--target=~nosuchuser/tmp/x`
// therefore becomes `/tmp/x`, and stow stows into it and exits 0. gostow leaves
// the token alone so that the caller's directory check rejects it.
//
// A test that only asserted "gostow does not expand it" would pass if gostow
// dropped the tilde entirely, so assert the exact text.
func TestExpandTildeLeavesAnUnknownUserAlone(t *testing.T) {
	const unknown = "nosuchuser99"
	// Not a skip. If this user somehow exists, the test below proves nothing, and
	// a test that proves nothing must fail rather than report success.
	if _, err := user.Lookup(unknown); err == nil {
		t.Fatalf("user %q exists on this machine, so this test would be vacuous", unknown)
	}

	for _, path := range []string{
		"~" + unknown + "/x",
		"~" + unknown + "/tmp/x",
		"~" + unknown,
	} {
		if got := expandTilde(path); got != path {
			t.Errorf("expandTilde(%q) = %q, want it unchanged (ledger PL-21)", path, got)
		}
	}
}

func TestExpandTildeExpandsHome(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)

	if got, want := expandTilde("~/x"), filepath.Join(home, "x"); got != want {
		t.Errorf("expandTilde(\"~/x\") = %q, want %q", got, want)
	}
	if got := expandTilde(`\~/x`); got != "~/x" {
		t.Errorf(`expandTilde("\\~/x") = %q, want "~/x"`, got)
	}
}

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

// Perl imposes no line-length limit on a config file, and a bufio.Scanner errors
// past 64 KiB. A long line must be read, not silently truncated to end-of-file
// and not rejected.
func TestStowrcReadsALineLongerThanAScannerBuffer(t *testing.T) {
	dir := t.TempDir()
	t.Chdir(dir)
	t.Setenv("HOME", t.TempDir())

	long := strings.Repeat("a", 100*1024)
	if err := os.WriteFile(filepath.Join(dir, ".stowrc"), []byte("--ignore="+long+"\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	tokens, err := readStowrcTokens(false)
	if err != nil {
		t.Fatalf("readStowrcTokens: %v", err)
	}
	if len(tokens) != 1 || tokens[0] != "--ignore="+long {
		t.Errorf("got %d token(s), first of length %d; want one token of length %d",
			len(tokens), len(tokens[0]), len("--ignore=")+len(long))
	}
}
