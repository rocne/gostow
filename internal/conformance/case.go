package conformance

import (
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// SandboxToken is the placeholder Case.Env entries may use for the sandbox root,
// whose real path is only known at run time. Normalize maps the path back to it.
const SandboxToken = "$SANDBOX"

// Case is one differential fixture. Cwd is relative to the sandbox root ("" is
// the root itself); stow cares where it is invoked from because .stowrc
// discovery and relative --dir/--target all key off it.
type Case struct {
	Name   string
	Stow   Tree
	Target Tree
	Args   []string
	// Env entries may contain SandboxToken. They are appended to a minimal
	// hermetic base environment, so a later HOME= wins over the default.
	Env []string
	// Rc is <root>/.stowrc — the one stow finds via the current directory.
	Rc string
	// HomeRc is <root>/home/.stowrc. stow reads both, home first, and
	// concatenates their tokens; see ledger PL-01.
	HomeRc string
	// Root is laid out at the sandbox root, beside stow/ and target/. Rc and
	// HomeRc cover the ordinary case of an rc *file*; this covers what neither
	// can express, such as a `.stowrc` that is a directory.
	Root Tree
	Cwd  string

	// Pre is an argv run with the *oracle* in both sandboxes before the measured
	// invocation, to build an already-stowed starting state. Hand-writing those
	// symlinks would be hand-writing the thing under test.
	Pre []string

	// FatalExitDiverges marks a case where stow's fatal exit status is
	// errno-derived and therefore undefined (ledger PL-07). stdout, stderr and
	// the tree are still compared byte-for-byte; only the status is exempt, and
	// gostow is required to exit 2.
	FatalExitDiverges bool

	// UsageOnStdout marks a case where the whole help block lands on stdout —
	// every usage error does this. gostow's help is written in gostow's own words
	// (SPEC §4.5), so the two blocks differ by design. stderr, the exit code and
	// the tree are still compared byte-for-byte; stdout is instead required to be
	// exactly what that same binary prints for --help.
	UsageOnStdout bool
}

// Materialize lays out the sandbox: a stow/ dir, a target/ dir, a home/ dir, and
// the optional rc files.
func (c Case) Materialize(root string) error {
	// All three exist even when empty: stow refuses a target that is not an
	// existing directory, and an absent HOME changes .stowrc discovery.
	for _, d := range []string{"stow", "target", "home"} {
		if err := os.MkdirAll(filepath.Join(root, d), 0o755); err != nil {
			return err
		}
	}
	if err := c.Stow.Materialize(filepath.Join(root, "stow")); err != nil {
		return err
	}
	if err := c.Target.Materialize(filepath.Join(root, "target")); err != nil {
		return err
	}
	if err := c.Root.Materialize(root); err != nil {
		return err
	}
	for path, content := range map[string]string{
		filepath.Join(root, ".stowrc"):         c.Rc,
		filepath.Join(root, "home", ".stowrc"): c.HomeRc,
	} {
		if content == "" {
			continue
		}
		if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
			return err
		}
	}
	return nil
}

// environ builds the environment the fixture runs under. HOME points inside the
// sandbox so that ~/.stowrc and ~/.stow-global-ignore are fixture-controlled
// rather than whatever the developer happens to have; PATH exists only because
// real stow is a Perl script whose shebang must resolve.
func (c Case) environ(root string) []string {
	env := []string{
		"HOME=" + filepath.Join(root, "home"),
		"PATH=/usr/local/bin:/usr/bin:/bin",
	}
	for _, e := range c.Env {
		env = append(env, expandSandbox(e, root))
	}
	return env
}

// expandSandbox substitutes the sandbox root for SandboxToken. Both sandboxes get
// the same argv text and each resolves it to its own root, so an absolute path
// can be a fixture without pinning it to one temp directory. Normalize puts the
// token back before the streams are compared.
func expandSandbox(s, root string) string {
	return strings.ReplaceAll(s, SandboxToken, root)
}

// argv returns Args with the sandbox root substituted.
func (c Case) argv(root string) []string { return expandAll(c.Args, root) }

func expandAll(in []string, root string) []string {
	if in == nil {
		return nil
	}
	out := make([]string, len(in))
	for i, s := range in {
		out[i] = expandSandbox(s, root)
	}
	return out
}

// Exec materialises the case into a fresh temp sandbox, runs bin from Cwd, and
// snapshots the whole sandbox afterwards so the resulting tree is part of the
// comparison.
func (c Case) Exec(t *testing.T, bin string) Run {
	t.Helper()
	run, _ := c.ExecAt(t, bin, t.TempDir())
	return run
}

// ExecAt is Exec with the sandbox root supplied and returned, for tests that
// must normalise the root out of the streams themselves — stow prints absolute
// paths at verbosity 2 and above.
func (c Case) ExecAt(t *testing.T, bin, root string) (Run, string) {
	t.Helper()
	RestorePermissionsOnCleanup(t, root)
	if err := c.Materialize(root); err != nil {
		t.Fatalf("materialize case %q: %v", c.Name, err)
	}
	run := RunBinary(bin, c.argv(root), c.environ(root), filepath.Join(root, c.Cwd))
	RestorePermissions(root)
	tree, err := Snapshot(root)
	if err != nil {
		t.Fatalf("snapshot case %q: %v", c.Name, err)
	}
	run.Tree = tree
	return run, root
}

// RestorePermissions makes every directory under root searchable again.
//
// A fixture that chmods a directory to 000, or drops a target's search bit, is
// exactly the shape needed to reproduce stow's errno-bearing fatal paths. It is
// also a directory that neither Snapshot nor os.RemoveAll can enter, so the mode
// has to come back off once the binary under test has exited. Nothing compared by
// the differential harness records a mode, so restoring one loses no evidence —
// and both sandboxes are restored identically.
func RestorePermissions(root string) {
	// WalkDir calls fn on a directory before reading it, so the chmod here
	// re-opens the door the fixture closed.
	_ = filepath.WalkDir(root, func(p string, d fs.DirEntry, err error) error {
		if err != nil || d == nil || !d.IsDir() {
			return nil
		}
		_ = os.Chmod(p, 0o755)
		return nil
	})
}

// RestorePermissionsOnCleanup is RestorePermissions run before t.TempDir's own
// cleanup, so a test that fails early still leaves a removable sandbox. Cleanups
// run last-registered-first, so this must be registered *after* the t.TempDir
// that created root.
func RestorePermissionsOnCleanup(t *testing.T, root string) {
	t.Helper()
	t.Cleanup(func() { RestorePermissions(root) })
}
