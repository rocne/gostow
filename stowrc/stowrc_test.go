package stowrc

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/rocne/gostow/stow"
)

// ParseFile is the whole pipeline over one named file: tokenize, parse, expand
// --dir/--target. This is the entry point a consumer slotting specific rc files
// uses, so the assertions cover the structured result end to end.
func TestParseFileReturnsTheStructuredOptionSet(t *testing.T) {
	t.Setenv("STOWRC_TEST_ROOT", "/opt/dots")
	home := t.TempDir()
	t.Setenv("HOME", home)

	path := filepath.Join(t.TempDir(), "rc")
	content := strings.Join([]string{
		`--dir=$STOWRC_TEST_ROOT/stow`,
		`--target ~/farm`,
		`-v2 --no-folding --dotfiles`,
		`--ignore='\.log' --ignore=tmp`,
		`--defer=man --override=info`,
		`-D oldpkg -S newpkg otherpkg`,
	}, "\n") + "\n"
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}

	p, err := ParseFile(path, false)
	if err != nil {
		t.Fatalf("ParseFile: %v", err)
	}
	if len(p.Errors) != 0 {
		t.Fatalf("Errors = %q, want none", p.Errors)
	}

	if p.Dir == nil || *p.Dir != "/opt/dots/stow" {
		t.Errorf("Dir = %v, want /opt/dots/stow ($VAR expanded)", str(p.Dir))
	}
	if p.Target == nil || *p.Target != filepath.Join(home, "farm") {
		t.Errorf("Target = %v, want %s (~ expanded)", str(p.Target), filepath.Join(home, "farm"))
	}
	if p.Verbosity() != 2 {
		t.Errorf("Verbosity() = %d, want 2", p.Verbosity())
	}
	if !p.NoFolding || !p.Dotfiles {
		t.Errorf("NoFolding = %v, Dotfiles = %v, want both true", p.NoFolding, p.Dotfiles)
	}
	if want := []string{`\.log`, "tmp"}; strings.Join(p.Ignore, " ") != strings.Join(want, " ") {
		t.Errorf("Ignore = %q, want %q", p.Ignore, want)
	}
	if len(p.Defer) != 1 || p.Defer[0] != "man" || len(p.Override) != 1 || p.Override[0] != "info" {
		t.Errorf("Defer = %q, Override = %q", p.Defer, p.Override)
	}

	// Package-name and action tokens are surfaced, not applied: stow discards
	// them for rc sources, and that stays the caller's call.
	want := []stow.Request{
		{Action: stow.ActionUnstow, Packages: []string{"oldpkg"}},
		{Action: stow.ActionStow, Packages: []string{"newpkg", "otherpkg"}},
	}
	if fmt.Sprint(p.Requests) != fmt.Sprint(want) {
		t.Errorf("Requests = %v, want %v", p.Requests, want)
	}
}

// A named file that fails to open is the caller's error, verbatim — the silent
// skip stow applies belongs to its home+cwd discovery, which stays caller-side.
func TestParseFileReturnsTheOpenError(t *testing.T) {
	_, err := ParseFile(filepath.Join(t.TempDir(), "absent"), false)
	if !errors.Is(err, os.ErrNotExist) {
		t.Errorf("err = %v, want an os.ErrNotExist", err)
	}
}

// A file that opens but cannot be read reproduces stow's Perl-readline failure
// shape: a bare die naming the file. A directory is the reachable instance —
// open(2) succeeds and the first read returns EISDIR — and it is unreadable to
// root as well, so unlike a chmod-000 file this test cannot silently skip.
func TestParseFileOnADirectoryDiesLikeStow(t *testing.T) {
	dir := filepath.Join(t.TempDir(), ".stowrc")
	if err := os.Mkdir(dir, 0o755); err != nil {
		t.Fatal(err)
	}

	_, err := ParseFile(dir, false)
	var de *DieError
	if !errors.As(err, &de) {
		t.Fatalf("error is %T (%v), want *DieError", err, err)
	}
	if want := "Could not close open file: " + dir; de.Msg != want {
		t.Errorf("DieError.Msg = %q, want %q", de.Msg, want)
	}
}

// The undefined-variable die surfaces through ParseFile with the option named,
// exactly as stow dies after parsing an rc file.
func TestParseFileDiesOnAnUndefinedVariable(t *testing.T) {
	path := filepath.Join(t.TempDir(), "rc")
	if err := os.WriteFile(path, []byte("--dir=$STOWRC_TEST_UNDEFINED\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	_, err := ParseFile(path, false)
	var de *DieError
	if !errors.As(err, &de) {
		t.Fatalf("error is %T (%v), want *DieError", err, err)
	}
	want := "--dir option references undefined environment variable $STOWRC_TEST_UNDEFINED; aborting!"
	if de.Msg != want {
		t.Errorf("DieError.Msg = %q, want %q", de.Msg, want)
	}
}

// PL-02 through the whole pipeline: without fixQuirks the option after '#' is
// honoured and the bare words land in Requests; with it the line ends at '#'.
func TestParseFileFixQuirksTogglesCommentHandling(t *testing.T) {
	path := filepath.Join(t.TempDir(), "rc")
	if err := os.WriteFile(path, []byte("--ignore=keep # --ignore=drop pkgword\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	quirky, err := ParseFile(path, false)
	if err != nil {
		t.Fatal(err)
	}
	if want := []string{"keep", "drop"}; strings.Join(quirky.Ignore, " ") != strings.Join(want, " ") {
		t.Errorf("without fixQuirks: Ignore = %q, want %q (the option after '#' is honoured)", quirky.Ignore, want)
	}
	if len(quirky.Requests) != 1 || strings.Join(quirky.Requests[0].Packages, " ") != "# pkgword" {
		t.Errorf("without fixQuirks: Requests = %v, want the bare words surfaced as package names", quirky.Requests)
	}

	fixed, err := ParseFile(path, true)
	if err != nil {
		t.Fatal(err)
	}
	if len(fixed.Ignore) != 1 || fixed.Ignore[0] != "keep" {
		t.Errorf("with fixQuirks: Ignore = %q, want [keep]", fixed.Ignore)
	}
	if len(fixed.Requests) != 0 {
		t.Errorf("with fixQuirks: Requests = %v, want none", fixed.Requests)
	}
}

func str(p *string) string {
	if p == nil {
		return "<nil>"
	}
	return *p
}
