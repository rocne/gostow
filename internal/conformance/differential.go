//go:build oracle

package conformance

import (
	"path/filepath"
	"strings"
	"testing"
)

// AssertSameAsOracle materialises c into two separate sandboxes, runs real stow
// on one and gostow on the other, normalises each sandbox's root out of the
// streams, and requires Stdout, Stderr, ExitCode and Tree to match. Any
// difference is a conformance bug — in gostow, or an unrecorded Parity Ledger
// entry.
func AssertSameAsOracle(t *testing.T, c Case) {
	t.Helper()

	// A parity fixture must be argv real stow could have been given. gostow's own
	// extensions are prefixed "gostow-" precisely so this guard can exist: using
	// one here would compare gostow against an oracle that never saw the flag,
	// and the suite would quietly stop testing parity. Forbid, do not filter.
	for _, arg := range append(append([]string{}, c.Args...), c.Pre...) {
		if strings.HasPrefix(arg, "--gostow-") {
			t.Fatalf("fixture %q passes the gostow extension %q to the parity suite; "+
				"extensions belong in the extension tests, never here", c.Name, arg)
		}
	}

	oracle := OraclePath(t)
	gostow := GostowPath(t)

	oracleRoot := t.TempDir()
	RestorePermissionsOnCleanup(t, oracleRoot)
	gostowRoot := t.TempDir()
	RestorePermissionsOnCleanup(t, gostowRoot)
	if err := c.Materialize(oracleRoot); err != nil {
		t.Fatalf("materialize oracle sandbox: %v", err)
	}
	if err := c.Materialize(gostowRoot); err != nil {
		t.Fatalf("materialize gostow sandbox: %v", err)
	}

	for _, root := range []string{oracleRoot, gostowRoot} {
		if c.Pre == nil {
			continue
		}
		pre := RunBinary(oracle, c.preArgv(root), c.environ(root), filepath.Join(root, c.Cwd))
		if pre.ExitCode != 0 {
			t.Fatalf("pre-stow in %s failed (%d): %s", root, pre.ExitCode, pre.Stderr)
		}
	}

	oRun := runIn(t, oracle, c, oracleRoot)
	gRun := runIn(t, gostow, c, gostowRoot)

	// One normalisation, tied to one ruled divergence: the identity banner
	// (PL-12). Nothing else about the streams is touched.
	clean := func(s, root string) string {
		return NormalizeIdentity(Normalize(s, root))
	}
	oRun.Stdout = clean(oRun.Stdout, oracleRoot)
	oRun.Stderr = clean(oRun.Stderr, oracleRoot)
	oRun.Tree = Normalize(oRun.Tree, oracleRoot)
	gRun.Stdout = clean(gRun.Stdout, gostowRoot)
	gRun.Stderr = clean(gRun.Stderr, gostowRoot)
	gRun.Tree = Normalize(gRun.Tree, gostowRoot)

	switch {
	case c.UsageOnStdout:
		// stow dumps its whole help block on a usage error. The *diagnostic* is
		// the contract — it goes to stderr and is compared byte for byte below —
		// while the help block is prose, and gostow's prose is its own (SPEC §4.5).
		//
		// So the claim is not "the two agree", which would be false, but the
		// stronger and still-checkable "each prints exactly the help it prints for
		// --help". A fixture that silently stopped printing usage would fail here
		// rather than pass by comparing nothing.
		assertIsOwnHelp(t, c, "oracle", oracle, oracleRoot, oRun.Stdout)
		assertIsOwnHelp(t, c, "gostow", gostow, gostowRoot, gRun.Stdout)
	case oRun.Stdout != gRun.Stdout:
		t.Errorf("stdout mismatch for %q\noracle:\n%s\ngostow:\n%s", c.Name, oRun.Stdout, gRun.Stdout)
	}
	if oRun.Stderr != gRun.Stderr {
		t.Errorf("stderr mismatch for %q\noracle:\n%s\ngostow:\n%s", c.Name, oRun.Stderr, gRun.Stderr)
	}
	switch {
	case c.FatalExitDiverges:
		if gRun.ExitCode != 2 {
			t.Errorf("%q: gostow exit = %d, want 2 (fatal errors are pinned; ledger PL-07)", c.Name, gRun.ExitCode)
		}
	case oRun.ExitCode != gRun.ExitCode:
		t.Errorf("exit code mismatch for %q: oracle=%d gostow=%d", c.Name, oRun.ExitCode, gRun.ExitCode)
	}
	if oRun.Tree != gRun.Tree {
		t.Errorf("tree mismatch for %q\noracle:\n%s\ngostow:\n%s", c.Name, oRun.Tree, gRun.Tree)
	}
}

// assertIsOwnHelp requires that stdout is exactly what `bin --help` prints, and
// that it is not empty.
func assertIsOwnHelp(t *testing.T, c Case, who, bin, root, stdout string) {
	t.Helper()

	help := RunBinary(bin, []string{"--help"}, c.environ(root), filepath.Join(root, c.Cwd))
	want := NormalizeIdentity(Normalize(help.Stdout, root))
	if strings.TrimSpace(want) == "" {
		t.Fatalf("%q: %s --help printed nothing; the check below would be vacuous", c.Name, who)
	}
	if stdout != want {
		t.Errorf("%q: %s printed a usage error whose stdout is not its own --help\ngot:\n%s\nwant:\n%s",
			c.Name, who, stdout, want)
	}
}

// preArgv returns Pre with the sandbox root substituted. It lives here, beside
// its only caller, because Pre is meaningful only when there is an oracle to
// build the starting state with.
func (c Case) preArgv(root string) []string { return expandAll(c.Pre, root) }

func runIn(t *testing.T, bin string, c Case, root string) Run {
	t.Helper()
	run := RunBinary(bin, c.argv(root), c.environ(root), filepath.Join(root, c.Cwd))
	RestorePermissions(root)
	tree, err := Snapshot(root)
	if err != nil {
		t.Fatalf("snapshot %q: %v", root, err)
	}
	run.Tree = tree
	return run
}
