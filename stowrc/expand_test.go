package stowrc

import (
	"errors"
	"os/user"
	"path/filepath"
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

func TestExpandFilepathExpandsVariables(t *testing.T) {
	t.Setenv("STOWRC_TEST_A", "alpha")
	t.Setenv("STOWRC_TEST_B", "beta")

	for in, want := range map[string]string{
		`/x/$STOWRC_TEST_A/y`:    "/x/alpha/y",
		`/x/${STOWRC_TEST_B}/y`:  "/x/beta/y",
		`/x/\$STOWRC_TEST_A/y`:   "/x/$STOWRC_TEST_A/y",
		`$STOWRC_TEST_A/$STOWRC_TEST_B`: "alpha/beta",
	} {
		got, err := ExpandFilepath(in, "--target option")
		if err != nil {
			t.Fatalf("ExpandFilepath(%q): %v", in, err)
		}
		if got != want {
			t.Errorf("ExpandFilepath(%q) = %q, want %q", in, got, want)
		}
	}
}

// An undefined variable is a bare Perl die(), byte for byte, and the option
// name is part of the message.
func TestExpandFilepathDiesOnAnUndefinedVariable(t *testing.T) {
	_, err := ExpandFilepath("/x/$STOWRC_TEST_UNDEFINED/y", "--target option")
	if err == nil {
		t.Fatal("ExpandFilepath returned no error for an undefined variable")
	}
	var de *DieError
	if !errors.As(err, &de) {
		t.Fatalf("error is %T, want *DieError", err)
	}
	want := "--target option references undefined environment variable $STOWRC_TEST_UNDEFINED; aborting!"
	if de.Msg != want {
		t.Errorf("DieError.Msg = %q, want %q", de.Msg, want)
	}
}
