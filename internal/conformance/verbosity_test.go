package conformance

import (
	"strings"
	"testing"
)

// PL-11 in its testable form: byte-exactness is owed at verbosity 0-2, and at
// any higher verbosity the *subsequence* of lines that levels 0-2 would have
// emitted must still appear, byte-for-byte and in order. The trace lines
// interleaved among them are unconstrained — they expose Perl's call structure,
// which gostow is deliberately free not to have.
//
// The 0-2 bytes themselves are pinned against real stow by TestCLIAgainstOracle;
// this test pins the containment, which no differential comparison can.
func TestVerbosityZeroToTwoIsASubsequenceOfHigherLevels(t *testing.T) {
	bin := GostowPath(t)

	cases := []Case{
		{
			Name: "fold then unfold",
			Stow: Tree{"one/sub/a": F("a"), "two/sub/b": F("b")},
			Args: []string{"-d", "stow", "-t", "target", "one", "two"},
		},
		{
			Name:   "conflict",
			Stow:   Tree{"pkg/f": F("x")},
			Target: Tree{"f": F("y")},
			Args:   []string{"-d", "stow", "-t", "target", "pkg"},
		},
		{
			Name: "simulate a dotfiles package",
			Stow: Tree{"pkg/dot-config/x": F("x")},
			Args: []string{"-d", "stow", "-t", "target", "-n", "--dotfiles", "pkg"},
		},
	}

	for _, c := range cases {
		t.Run(c.Name, func(t *testing.T) {
			base := c
			base.Args = append(append([]string{}, c.Args...), "-vv")
			lowRun, lowRoot := base.ExecAt(t, bin, t.TempDir())
			// Levels 1-2 print the sandbox's absolute path, and each run gets a
			// fresh one, so each stream is normalised against its own root.
			lowErr := Normalize(lowRun.Stderr, lowRoot)

			for _, level := range []string{"-vvv", "-vvvv", "-vvvvv"} {
				high := c
				high.Args = append(append([]string{}, c.Args...), level)
				gotRun, gotRoot := high.ExecAt(t, bin, t.TempDir())
				got := Run{ExitCode: gotRun.ExitCode, Stderr: Normalize(gotRun.Stderr, gotRoot)}
				low := Run{ExitCode: lowRun.ExitCode, Stderr: lowErr}

				if got.ExitCode != low.ExitCode {
					t.Errorf("%s: exit = %d, want %d", level, got.ExitCode, low.ExitCode)
				}
				if missing, ok := isSubsequence(lines(low.Stderr), lines(got.Stderr)); !ok {
					t.Errorf("%s dropped a level-0-2 line: %q\n--- at -vv ---\n%s--- at %s ---\n%s",
						level, missing, low.Stderr, level, got.Stderr)
				}
			}
		})
	}
}

func lines(s string) []string {
	s = strings.TrimSuffix(s, "\n")
	if s == "" {
		return nil
	}
	return strings.Split(s, "\n")
}

// isSubsequence reports whether want appears within got in order, and the first
// line that did not.
func isSubsequence(want, got []string) (string, bool) {
	i := 0
	for _, g := range got {
		if i < len(want) && g == want[i] {
			i++
		}
	}
	if i < len(want) {
		return want[i], false
	}
	return "", true
}
