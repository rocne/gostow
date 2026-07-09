package conformance

import (
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
	Cwd    string
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
		env = append(env, strings.ReplaceAll(e, SandboxToken, root))
	}
	return env
}

// Exec materialises the case into a fresh temp sandbox, runs bin from Cwd, and
// snapshots the whole sandbox afterwards so the resulting tree is part of the
// comparison.
func (c Case) Exec(t *testing.T, bin string) Run {
	t.Helper()
	root := t.TempDir()
	if err := c.Materialize(root); err != nil {
		t.Fatalf("materialize case %q: %v", c.Name, err)
	}
	run := RunBinary(bin, c.Args, c.environ(root), filepath.Join(root, c.Cwd))
	tree, err := Snapshot(root)
	if err != nil {
		t.Fatalf("snapshot case %q: %v", c.Name, err)
	}
	run.Tree = tree
	return run
}
