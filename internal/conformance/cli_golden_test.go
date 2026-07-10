package conformance

import (
	"path/filepath"
	"testing"
)

// Layer 5. The same fixtures the differential suite runs, checked against what
// real stow 2.4.1 did the last time an oracle was present — no Perl, no stow, no
// network. This is what `go test ./...` runs.
//
// It cannot notice that stow's behaviour has changed; only the oracle job can,
// and it runs on every PR. What it does notice is a gostow that has changed,
// which is the thing a contributor edits.
func TestCLIAgainstGoldens(t *testing.T) {
	gostow := GostowPath(t)

	for _, c := range cliCases() {
		t.Run(c.Name, func(t *testing.T) {
			want := LoadGolden(t, c.Name)

			root := t.TempDir()
			RestorePermissionsOnCleanup(t, root)
			if err := c.Materialize(root); err != nil {
				t.Fatalf("materialize sandbox: %v", err)
			}

			// The starting state for a Pre case is built by gostow here, because
			// there is no oracle to build it with and Snapshot hashes file content
			// rather than recording it, so the oracle's post-Pre tree cannot be
			// replayed. A gostow whose *stow* is wrong could therefore hide a bug
			// in the measured unstow. The differential suite builds the same state
			// with real stow and would catch exactly that, on every PR.
			if c.Pre != nil {
				pre := RunBinary(gostow, expandAll(c.Pre, root), c.environ(root), filepath.Join(root, c.Cwd))
				if pre.ExitCode != 0 {
					t.Fatalf("pre-stow failed (%d): %s", pre.ExitCode, pre.Stderr)
				}
			}

			got := runGostow(t, gostow, c, root)
			AssertMatches(t, c, want, got, gostow, root)
		})
	}
}

func runGostow(t *testing.T, bin string, c Case, root string) Run {
	t.Helper()

	run := RunBinary(bin, c.argv(root), c.environ(root), filepath.Join(root, c.Cwd))
	RestorePermissions(root)
	tree, err := Snapshot(root)
	if err != nil {
		t.Fatalf("snapshot %q: %v", root, err)
	}
	run.Tree = tree
	run.Stdout = NormalizeIdentity(Normalize(run.Stdout, root))
	run.Stderr = NormalizeIdentity(Normalize(run.Stderr, root))
	run.Tree = Normalize(run.Tree, root)
	return run
}

// Every case must have a golden, and every golden must have a case. Without the
// second half, deleting a fixture leaves its golden behind and the layer slowly
// becomes a museum; without the first, a new fixture is silently untested by the
// hermetic suite.
func TestEveryCaseHasAGoldenAndViceVersa(t *testing.T) {
	cases := cliCases()
	if len(cases) == 0 {
		t.Fatal("no cases; this test would be vacuous")
	}

	wanted := map[string]string{}
	for _, c := range cases {
		name := GoldenName(c.Name)
		if prior, dup := wanted[name]; dup {
			t.Fatalf("cases %q and %q both slug to %q; one golden would overwrite the other",
				prior, c.Name, name)
		}
		wanted[name] = c.Name
	}

	found, err := filepath.Glob(filepath.Join(goldenDir, "*.txt"))
	if err != nil {
		t.Fatal(err)
	}
	have := map[string]bool{}
	for _, path := range found {
		have[filepath.Base(path[:len(path)-len(".txt")])] = true
	}

	for name, caseName := range wanted {
		if !have[name] {
			t.Errorf("case %q has no golden at %s", caseName, GoldenPath(caseName))
		}
	}
	for name := range have {
		if _, ok := wanted[name]; !ok {
			t.Errorf("golden %s.txt has no case; delete it or restore the fixture", name)
		}
	}
	t.Logf("%d fixtures, %d goldens", len(wanted), len(have))
}

func TestGoldenRoundTrips(t *testing.T) {
	g := Golden{ExitCode: 1, Stdout: "a\nb\n", Stderr: "boom\n", Tree: "dir  stow\n"}
	encoded, err := Encode(g)
	if err != nil {
		t.Fatal(err)
	}
	got, err := Decode(encoded)
	if err != nil {
		t.Fatal(err)
	}
	if got != g {
		t.Errorf("round trip changed the golden:\n got %#v\nwant %#v", got, g)
	}

	// Empty sections are the common case (a silent, successful stow) and must
	// survive the trip.
	empty := Golden{}
	encoded, err = Encode(empty)
	if err != nil {
		t.Fatal(err)
	}
	if got, err = Decode(encoded); err != nil || got != empty {
		t.Errorf("empty golden did not round trip: %#v, %v", got, err)
	}

	if _, err := Encode(Golden{Stdout: "### tree\n"}); err == nil {
		t.Error("Encode accepted a section containing the format's own marker")
	}
	if _, err := Decode("garbage"); err == nil {
		t.Error("Decode accepted garbage")
	}
}
