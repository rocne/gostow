//go:build oracle

package conformance

import (
	"path/filepath"
	"testing"
)

// AssertSameAsOracle materialises c into two separate sandboxes, runs real stow
// on one and gostow on the other, normalises each sandbox's root out of the
// streams, and requires Stdout, Stderr, ExitCode and Tree to match. Any
// difference is a conformance bug — in gostow, or an unrecorded Parity Ledger
// entry.
func AssertSameAsOracle(t *testing.T, c Case) {
	t.Helper()

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

	oRun := runIn(t, oracle, c, oracleRoot)
	gRun := runIn(t, gostow, c, gostowRoot)

	oRun.Stdout = Normalize(oRun.Stdout, oracleRoot)
	oRun.Stderr = Normalize(oRun.Stderr, oracleRoot)
	oRun.Tree = Normalize(oRun.Tree, oracleRoot)
	gRun.Stdout = Normalize(gRun.Stdout, gostowRoot)
	gRun.Stderr = Normalize(gRun.Stderr, gostowRoot)
	gRun.Tree = Normalize(gRun.Tree, gostowRoot)

	if oRun.Stdout != gRun.Stdout {
		t.Errorf("stdout mismatch for %q\noracle:\n%s\ngostow:\n%s", c.Name, oRun.Stdout, gRun.Stdout)
	}
	if oRun.Stderr != gRun.Stderr {
		t.Errorf("stderr mismatch for %q\noracle:\n%s\ngostow:\n%s", c.Name, oRun.Stderr, gRun.Stderr)
	}
	if oRun.ExitCode != gRun.ExitCode {
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
