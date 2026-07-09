//go:build oracle

package conformance

import "testing"

// Proves the harness can drive real stow end to end. gostow has no engine yet,
// so this deliberately does not compare the two — it only asserts the oracle
// plans a trivial one-file stow cleanly (dry run, exit 0).
func TestOracleSmoke(t *testing.T) {
	bin := OraclePath(t)
	c := Case{
		Name: "oracle-smoke",
		Stow: Tree{"pkg/file": F("hello")},
		Args: []string{"-n", "-v", "-d", "stow", "-t", "target", "pkg"},
	}
	run := c.Exec(t, bin)
	if run.ExitCode != 0 {
		t.Fatalf("oracle exit %d\nstdout:\n%s\nstderr:\n%s", run.ExitCode, run.Stdout, run.Stderr)
	}
}
