package conformance

import (
	"path/filepath"
	"strings"
	"testing"
)

func TestSnapshotDeterministicAndVerbatimLinks(t *testing.T) {
	tree := Tree{
		"b/file":   F("hello"),
		"a":        D(),
		"b":        D(),
		"link":     L("../does/not/exist"),
		"selflink": L("selflink"),
	}
	root := t.TempDir()
	if err := tree.Materialize(root); err != nil {
		t.Fatalf("materialize: %v", err)
	}

	first, err := Snapshot(root)
	if err != nil {
		t.Fatalf("snapshot: %v", err)
	}
	second, err := Snapshot(root)
	if err != nil {
		t.Fatalf("snapshot: %v", err)
	}
	if first != second {
		t.Fatalf("snapshot not deterministic:\n%s\n---\n%s", first, second)
	}

	// The dangling target is recorded as written, never resolved or followed.
	if !strings.Contains(first, "link link -> ../does/not/exist\n") {
		t.Errorf("verbatim symlink target missing:\n%s", first)
	}
	if !strings.Contains(first, "link selflink -> selflink\n") {
		t.Errorf("self-referential symlink not recorded verbatim:\n%s", first)
	}

	lines := strings.Split(strings.TrimRight(first, "\n"), "\n")
	sorted := append([]string(nil), lines...)
	for i := 1; i < len(sorted); i++ {
		if sorted[i-1] > sorted[i] {
			t.Errorf("snapshot not byte-sorted at line %d:\n%s", i, first)
		}
	}
}

func TestTreeMaterializeRoundTrips(t *testing.T) {
	tree := Tree{
		"dir":       D(),
		"dir/inner": F("content"),
		"top":       F("x"),
	}
	root := t.TempDir()
	if err := tree.Materialize(root); err != nil {
		t.Fatalf("materialize: %v", err)
	}
	snap, err := Snapshot(root)
	if err != nil {
		t.Fatalf("snapshot: %v", err)
	}
	for _, want := range []string{"dir  dir\n", "dir/inner", "file top"} {
		if !strings.Contains(snap, want) {
			t.Errorf("round-trip missing %q:\n%s", want, snap)
		}
	}
}

func TestNormalizeReplacesRoot(t *testing.T) {
	root := t.TempDir()
	line := "LINK: " + filepath.Join(root, "target", "file") + " => stow"
	got := Normalize(line, root)
	if strings.Contains(got, root) {
		t.Errorf("root not replaced: %q", got)
	}
	if !strings.Contains(got, "$SANDBOX") {
		t.Errorf("token missing: %q", got)
	}
}

func TestRunBinaryCapturesNonzeroExit(t *testing.T) {
	run := RunBinary("sh", []string{"-c", "exit 3"}, nil, t.TempDir())
	if run.ExitCode != 3 {
		t.Errorf("exit code: got %d, want 3", run.ExitCode)
	}
}

// A nil env must mean an empty environment, not an inherited one. Go's exec
// inherits on nil, which would let the developer's ~/.stowrc and
// ~/.stow-global-ignore leak into every fixture and quietly rewrite the spec.
func TestRunBinaryDoesNotInheritEnvironment(t *testing.T) {
	t.Setenv("GOSTOW_LEAK_CANARY", "leaked")
	// printf, not `echo -n`: /bin/sh is dash on the CI runners, where `echo -n`
	// prints the flag.
	run := RunBinary("/bin/sh", []string{"-c", `printf '%s' "${GOSTOW_LEAK_CANARY-unset}"`}, nil, t.TempDir())
	if run.Stdout != "unset" {
		t.Errorf("RunBinary leaked the parent environment: got %q, want %q", run.Stdout, "unset")
	}
}

// Case.environ pins HOME inside the sandbox, and expands the sandbox token.
func TestCaseEnvironIsHermetic(t *testing.T) {
	c := Case{Env: []string{"STOW_DIR=" + SandboxToken + "/stow"}}
	env := c.environ("/sandbox")
	want := []string{"HOME=/sandbox/home", "PATH=/usr/local/bin:/usr/bin:/bin", "STOW_DIR=/sandbox/stow"}
	if len(env) != len(want) {
		t.Fatalf("environ = %q, want %q", env, want)
	}
	for i := range want {
		if env[i] != want[i] {
			t.Errorf("environ[%d] = %q, want %q", i, env[i], want[i])
		}
	}
}
