//go:build oracle

package conformance

import (
	"flag"
	"path/filepath"
	"strings"
	"testing"
)

// updateGoldens regenerates the layer-5 recordings from the live oracle. It is
// declared only under the `oracle` build tag, so the hermetic suite cannot
// rewrite the answers it is being graded against — the flag does not exist there.
//
// OraclePath asserts the binary reports exactly 2.4.1 before any of this runs, so
// a golden can only ever be a recording of the pinned referent.
var updateGoldens = flag.Bool("update-goldens", false,
	"rewrite testdata/goldens from the pinned stow 2.4.1 oracle")

// AssertSameAsOracle materialises c into two separate sandboxes, runs real stow on
// one and gostow on the other, normalises each sandbox's root out of the streams,
// and compares them. Any difference is a conformance bug — in gostow, or an
// unrecorded Parity Ledger entry.
//
// It also records what the oracle did, so `go test ./...` can check the same
// claim on a machine with no Perl. See AssertMatches for the three narrow
// exemptions, and docs/TEST-PLAN.md §2 for why goldens and a live oracle are only
// worth anything together.
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

	want := Golden{ExitCode: oRun.ExitCode, Stdout: oRun.Stdout, Stderr: oRun.Stderr, Tree: oRun.Tree}

	// The oracle's own stdout is checked here rather than in AssertMatches: the
	// golden layer has no oracle to ask, and asserting it against a recording
	// would be circular.
	if c.UsageOnStdout {
		assertIsOwnHelp(t, c, "oracle", oracle, oracleRoot, oRun.Stdout)
	}

	if *updateGoldens {
		if err := SaveGolden(c.Name, want); err != nil {
			t.Fatalf("write golden for %q: %v", c.Name, err)
		}
	}

	AssertMatches(t, c, want, gRun, gostow, gostowRoot)
}

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

// preArgv returns Pre with the sandbox root substituted. It lives here, beside
// its only caller, because Pre is meaningful only when there is an oracle to
// build the starting state with.
func (c Case) preArgv(root string) []string { return expandAll(c.Pre, root) }
