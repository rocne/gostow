//go:build oracle

package stow

import (
	"bytes"
	"errors"
	"path/filepath"
	"testing"

	"github.com/rocne/gostow/internal/conformance"
)

// engineCase drives the same fixture through real stow and through Apply, and
// requires the resulting trees — and the operation log — to be identical.
//
// The log comparison runs at verbosity 1, where stow emits nothing but the
// LINK/UNLINK/MKDIR/RMDIR/MV lines. Verbosity 2 adds planning chatter that
// mentions absolute paths; that is compared elsewhere. Levels 3+ are not a
// byte-parity contract at all (ledger PL-11).
type engineCase struct {
	name   string
	stow   conformance.Tree
	target conformance.Tree
	// pre is an argv run with the *oracle* in both sandboxes before the
	// measured operation, to build an already-stowed starting state. Writing
	// those symlinks by hand would be hand-writing the thing under test.
	pre      []string
	args     []string // stow's argv after "-d stow -t target"
	opts     Options  // the equivalent Options; Dir/Target/Log/Verbosity are filled in
	reqs     []Request
	conflict bool // planning is expected to abort with a ConflictError
}

func runEngineCase(t *testing.T, c engineCase) {
	t.Helper()
	oracle := conformance.OraclePath(t)

	// Real stow, in its own sandbox.
	oracleRoot := t.TempDir()
	cc := conformance.Case{Name: c.name, Stow: c.stow, Target: c.target}
	if err := cc.Materialize(oracleRoot); err != nil {
		t.Fatalf("materialize: %v", err)
	}
	// gostow's engine, in its own sandbox.
	gostowRoot := t.TempDir()
	if err := cc.Materialize(gostowRoot); err != nil {
		t.Fatalf("materialize: %v", err)
	}

	if c.pre != nil {
		for _, root := range []string{oracleRoot, gostowRoot} {
			pre := conformance.RunBinary(oracle, append([]string{"-d", "stow", "-t", "target"}, c.pre...),
				oracleEnv(root), root)
			if pre.ExitCode != 0 {
				t.Fatalf("pre-stow in %s failed (%d): %s", root, pre.ExitCode, pre.Stderr)
			}
		}
	}

	args := append([]string{"-d", "stow", "-t", "target", "-v", "1"}, c.args...)
	oRun := conformance.RunBinary(oracle, args, oracleEnv(oracleRoot), oracleRoot)
	oTree, err := conformance.Snapshot(oracleRoot)
	if err != nil {
		t.Fatalf("snapshot oracle: %v", err)
	}

	t.Setenv("HOME", filepath.Join(gostowRoot, "home"))

	var log bytes.Buffer
	opts := c.opts
	opts.Dir = filepath.Join(gostowRoot, "stow")
	opts.Target = filepath.Join(gostowRoot, "target")
	opts.Verbosity = 1
	opts.Log = &log
	_, applyErr := Apply(opts, c.reqs...)

	gTree, err := conformance.Snapshot(gostowRoot)
	if err != nil {
		t.Fatalf("snapshot gostow: %v", err)
	}

	var conflictErr *ConflictError
	gotConflict := errors.As(applyErr, &conflictErr)
	if !gotConflict && applyErr != nil {
		t.Fatalf("Apply: unexpected error: %v", applyErr)
	}
	if gotConflict != c.conflict {
		t.Errorf("conflict = %v, want %v (oracle exit %d, stderr: %s)",
			gotConflict, c.conflict, oRun.ExitCode, oRun.Stderr)
	}
	if wantExit := boolExit(c.conflict); oRun.ExitCode != wantExit {
		t.Errorf("oracle exit = %d, want %d; stderr: %s", oRun.ExitCode, wantExit, oRun.Stderr)
	}

	if got, want := conformance.Normalize(gTree, gostowRoot), conformance.Normalize(oTree, oracleRoot); got != want {
		t.Errorf("tree mismatch\n--- oracle ---\n%s--- gostow ---\n%s", want, got)
	}

	// stow prints conflicts from the CLI layer, not the engine, so only compare
	// the operation log on the non-conflicting cases.
	if !c.conflict {
		wantLog := conformance.Normalize(oRun.Stderr, oracleRoot)
		gotLog := conformance.Normalize(log.String(), gostowRoot)
		if gotLog != wantLog {
			t.Errorf("log mismatch\n--- oracle ---\n%s--- gostow ---\n%s", wantLog, gotLog)
		}
	}
}

func oracleEnv(root string) []string {
	return []string{"HOME=" + filepath.Join(root, "home"), "PATH=/usr/local/bin:/usr/bin:/bin"}
}

func boolExit(conflict bool) int {
	if conflict {
		return 1
	}
	return 0
}

func stowReq(pkgs ...string) []Request { return []Request{{Action: ActionStow, Packages: pkgs}} }
func unstowReq(pkgs ...string) []Request {
	return []Request{{Action: ActionUnstow, Packages: pkgs}}
}

func TestEngineAgainstOracle(t *testing.T) {
	F, D, L := conformance.F, conformance.D, conformance.L

	cases := []engineCase{
		{
			name: "single file into empty target",
			stow: conformance.Tree{"pkg/f": F("hello\n")},
			args: []string{"pkg"},
			opts: Options{Fold: true},
			reqs: stowReq("pkg"),
		},
		{
			name: "tree folding: a whole subdir becomes one link",
			stow: conformance.Tree{"pkg/sub/a": F("a"), "pkg/sub/b": F("b")},
			args: []string{"pkg"},
			opts: Options{Fold: true},
			reqs: stowReq("pkg"),
		},
		{
			name: "no folding: real dirs, one link per file",
			stow: conformance.Tree{"pkg/sub/a": F("a"), "pkg/sub/b": F("b")},
			args: []string{"--no-folding", "pkg"},
			opts: Options{Fold: false},
			reqs: stowReq("pkg"),
		},
		{
			name:     "conflict: existing plain file, nothing written",
			stow:     conformance.Tree{"pkg/f": F("new")},
			target:   conformance.Tree{"f": F("old")},
			args:     []string{"pkg"},
			opts:     Options{Fold: true},
			reqs:     stowReq("pkg"),
			conflict: true,
		},
		{
			name:   "recurse into an existing real directory",
			stow:   conformance.Tree{"pkg/sub/a": F("a")},
			target: conformance.Tree{"sub": D()},
			args:   []string{"pkg"},
			opts:   Options{Fold: true},
			reqs:   stowReq("pkg"),
		},
		{
			name: "unfold: second package splits open a folded dir",
			stow: conformance.Tree{"one/sub/a": F("a"), "two/sub/b": F("b")},
			args: []string{"one", "two"},
			opts: Options{Fold: true},
			reqs: stowReq("one", "two"),
		},
		{
			name: "built-in ignore list skips README and .git",
			stow: conformance.Tree{
				"pkg/f":          F("f"),
				"pkg/README.md":  F("readme"),
				"pkg/.gitignore": F("ignored"),
			},
			args: []string{"pkg"},
			opts: Options{Fold: true},
			reqs: stowReq("pkg"),
		},
		{
			name: "dotfiles translation",
			stow: conformance.Tree{"pkg/dot-bashrc": F("rc"), "pkg/dot-": F("x"), "pkg/notdot-y": F("y")},
			args: []string{"--dotfiles", "pkg"},
			opts: Options{Fold: true, Dotfiles: true},
			reqs: stowReq("pkg"),
		},
		{
			name:     "absolute symlink in package is a conflict",
			stow:     conformance.Tree{"pkg/abs": L("/etc/hostname")},
			args:     []string{"pkg"},
			opts:     Options{Fold: true},
			reqs:     stowReq("pkg"),
			conflict: true,
		},
	}

	cases = append(cases, unstowCases(F, D, L)...)

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) { runEngineCase(t, c) })
	}
}

// These all start from a target that real stow has already populated.
func unstowCases(F func(string) conformance.Node, D func() conformance.Node, L func(string) conformance.Node) []engineCase {
	return []engineCase{
		{
			name: "unstow a single file",
			stow: conformance.Tree{"pkg/f": F("hello\n")},
			pre:  []string{"pkg"},
			args: []string{"-D", "pkg"},
			opts: Options{Fold: true},
			reqs: unstowReq("pkg"),
		},
		{
			name: "unstow a folded tree",
			stow: conformance.Tree{"pkg/sub/a": F("a"), "pkg/sub/b": F("b")},
			pre:  []string{"pkg"},
			args: []string{"-D", "pkg"},
			opts: Options{Fold: true},
			reqs: unstowReq("pkg"),
		},
		{
			name: "refold: unstowing one of two packages collapses the dir again",
			stow: conformance.Tree{"one/sub/a": F("a"), "two/sub/b": F("b")},
			pre:  []string{"one", "two"},
			args: []string{"-D", "two"},
			opts: Options{Fold: true},
			reqs: unstowReq("two"),
		},
		{
			name: "restow is unstow then stow",
			stow: conformance.Tree{"pkg/sub/a": F("a")},
			pre:  []string{"pkg"},
			args: []string{"-R", "pkg"},
			opts: Options{Fold: true},
			reqs: []Request{{Action: ActionRestow, Packages: []string{"pkg"}}},
		},
		{
			name:   "unstow leaves an unowned link alone",
			stow:   conformance.Tree{"pkg/f": F("f")},
			target: conformance.Tree{"other": L("/etc/hostname")},
			pre:    []string{"pkg"},
			args:   []string{"-D", "pkg"},
			opts:   Options{Fold: true},
			reqs:   unstowReq("pkg"),
		},
		{
			name: "cleanup_invalid_links removes an orphaned stow-owned link",
			stow: conformance.Tree{"pkg/sub/a": F("a"), "pkg/sub/b": F("b")},
			pre:  []string{"--no-folding", "pkg"},
			// "gone" dangles into the package: stow owns it, and it blocks refolding.
			target: conformance.Tree{"sub/gone": L("../../stow/pkg/sub/gone")},
			args:   []string{"-D", "pkg"},
			opts:   Options{Fold: true},
			reqs:   unstowReq("pkg"),
		},
		{
			name:     "conflict: target stowed to a different package",
			stow:     conformance.Tree{"one/f": F("1"), "two/f": F("2")},
			pre:      []string{"one"},
			args:     []string{"two"},
			opts:     Options{Fold: true},
			reqs:     stowReq("two"),
			conflict: true,
		},
		{
			name: "--override relinks a foreign-owned target",
			stow: conformance.Tree{"one/f": F("1"), "two/f": F("2")},
			pre:  []string{"one"},
			args: []string{"--override=f", "two"},
			opts: Options{Fold: true, Override: []string{"f"}},
			reqs: stowReq("two"),
		},
		{
			name: "--defer skips a foreign-owned target",
			stow: conformance.Tree{"one/f": F("1"), "two/f": F("2")},
			pre:  []string{"one"},
			args: []string{"--defer=f", "two"},
			opts: Options{Fold: true, Defer: []string{"f"}},
			reqs: stowReq("two"),
		},
		{
			name:   "--adopt moves a conflicting plain file into the package",
			stow:   conformance.Tree{"pkg/f": F("pkgcontent")},
			target: conformance.Tree{"f": F("targetcontent")},
			args:   []string{"--adopt", "pkg"},
			opts:   Options{Fold: true, Adopt: true},
			reqs:   stowReq("pkg"),
		},
		{
			name: "stowing over an identical link is a no-op",
			stow: conformance.Tree{"pkg/f": F("f")},
			pre:  []string{"pkg"},
			args: []string{"pkg"},
			opts: Options{Fold: true},
			reqs: stowReq("pkg"),
		},
	}
}
