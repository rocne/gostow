package conformance

import (
	"path/filepath"
	"strings"
	"testing"
)

// AssertMatches compares one gostow run against what real stow did — whether
// "what real stow did" arrives from the live oracle (layer 4) or from a golden it
// recorded earlier (layer 5).
//
// It lives here, called by both, because the exemptions are the interesting part
// and a second copy of them would drift. Each exemption is narrow, named on the
// Case, and pays for itself by pinning something else in exchange:
//
//   - UsageOnStdout: the help block is gostow's own prose, so instead of matching
//     stow's bytes, gostow's stdout must equal gostow's own --help exactly.
//   - DiagnosticLinesOnly: the diagnostic is Perl's regex engine quoting bin/stow's
//     line numbers, so the number of diagnostics is compared instead of their text.
//   - FatalExitDiverges: stow's fatal status is errno-derived, so gostow is
//     required to exit 2 rather than to match it.
//
// Nothing else is exempt. stdout, stderr, the exit code and the resulting tree are
// bytes.
func AssertMatches(t *testing.T, c Case, want Golden, got Run, gostowBin, gostowRoot string) {
	t.Helper()

	switch {
	case c.UsageOnStdout:
		assertIsOwnHelp(t, c, "gostow", gostowBin, gostowRoot, got.Stdout)
	case want.Stdout != got.Stdout:
		t.Errorf("stdout mismatch for %q\nstow:\n%s\ngostow:\n%s", c.Name, want.Stdout, got.Stdout)
	}

	switch {
	case c.DiagnosticLinesOnly:
		assertSameDiagnosticCount(t, c, want.Stderr, got.Stderr)
	case want.Stderr != got.Stderr:
		t.Errorf("stderr mismatch for %q\nstow:\n%s\ngostow:\n%s", c.Name, want.Stderr, got.Stderr)
	}

	switch {
	case c.FatalExitDiverges:
		if got.ExitCode != 2 {
			t.Errorf("%q: gostow exit = %d, want 2 (fatal errors are pinned; ledger PL-07)", c.Name, got.ExitCode)
		}
	case want.ExitCode != got.ExitCode:
		t.Errorf("exit code mismatch for %q: stow=%d gostow=%d", c.Name, want.ExitCode, got.ExitCode)
	}

	if want.Tree != got.Tree {
		t.Errorf("tree mismatch for %q\nstow:\n%s\ngostow:\n%s", c.Name, want.Tree, got.Tree)
	}
}

// assertSameDiagnosticCount requires both binaries to have complained the same
// number of times, and at least once. The wording is Perl's; the count is the
// behaviour — Getopt::Long catches each callback's die and carries on, so two bad
// patterns produce two diagnostics.
func assertSameDiagnosticCount(t *testing.T, c Case, stowErr, gostowErr string) {
	t.Helper()

	count := func(s string) int {
		n := 0
		for _, line := range strings.Split(s, "\n") {
			if strings.TrimSpace(line) != "" {
				n++
			}
		}
		return n
	}
	o, g := count(stowErr), count(gostowErr)
	if o == 0 {
		t.Fatalf("%q: stow printed no diagnostic; the check below would be vacuous", c.Name)
	}
	if o != g {
		t.Errorf("%q: stow printed %d diagnostic line(s), gostow printed %d\nstow:\n%s\ngostow:\n%s",
			c.Name, o, g, stowErr, gostowErr)
	}
}

// assertIsOwnHelp requires that stdout is exactly what `bin --help` prints, and
// that it is not empty. A fixture that silently stopped printing usage would fail
// here rather than pass by comparing nothing.
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
