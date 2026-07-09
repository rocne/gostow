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
	gostowRoot := t.TempDir()
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
		pre := RunBinary(oracle, c.Pre, c.environ(root), filepath.Join(root, c.Cwd))
		if pre.ExitCode != 0 {
			t.Fatalf("pre-stow in %s failed (%d): %s", root, pre.ExitCode, pre.Stderr)
		}
	}

	oRun := runIn(t, oracle, c, oracleRoot)
	gRun := runIn(t, gostow, c, gostowRoot)

	oRun.Stdout = NormalizeIdentity(Normalize(oRun.Stdout, oracleRoot))
	oRun.Stderr = NormalizeIdentity(Normalize(oRun.Stderr, oracleRoot))
	oRun.Tree = Normalize(oRun.Tree, oracleRoot)
	gRun.Stdout = NormalizeIdentity(Normalize(gRun.Stdout, gostowRoot))
	gRun.Stderr = NormalizeIdentity(Normalize(gRun.Stderr, gostowRoot))
	gRun.Tree = Normalize(gRun.Tree, gostowRoot)

	if oRun.Stdout != gRun.Stdout {
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

func runIn(t *testing.T, bin string, c Case, root string) Run {
	t.Helper()
	run := RunBinary(bin, c.Args, c.environ(root), filepath.Join(root, c.Cwd))
	tree, err := Snapshot(root)
	if err != nil {
		t.Fatalf("snapshot %q: %v", root, err)
	}
	run.Tree = tree
	return run
}
