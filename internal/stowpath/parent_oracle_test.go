//go:build oracle

package stowpath

import (
	"os/exec"
	"strings"
	"testing"

	"github.com/rocne/gostow/internal/conformance"
)

// Parent's expectations in path_test.go are hand-written, and a hand-written
// expectation is a transcription — which is exactly how the bug this package
// exists to fix was born. internal/cli's `parentOf` was transcribed from the same
// Perl, disagreed with it on "/tmp", and nothing noticed because nothing asked
// Perl.
//
// So ask Perl. Stow::Util::parent is the referent; every case below is compared
// against it, including the ones no fixture would think to write.
func TestParentAgreesWithStowUtil(t *testing.T) {
	lib := conformance.OraclePerlLib(t, conformance.OraclePath(t))
	conformance.TrackOracleInput(t, conformance.PerlModulePath(t, lib, "Stow::Util"))

	paths := []string{
		"", "/", "//", "///",
		"a", "/a", "a/", "/a/", "//a", "//a//",
		"a/b", "/a/b", "a/b/", "/a/b/",
		"a/b/c", "/a/b/c", "a/b/c/", "/////a///b///c///",
		"tmp", "/tmp", "/tmp/", "//tmp", "/tmp/x", "/usr/local/stow",
		".", "./", "..", "../..", "./a", "a/./b",
		"a//b", "a///b///c",
		" ", "a b/c", "a\tb/c",
		"-", "/-", "0", "/0",
	}

	want := parentOracle(t, lib, paths)
	for i, p := range paths {
		if got := Parent(p); got != want[i] {
			t.Errorf("Parent(%q) = %q, Stow::Util::parent says %q", p, got, want[i])
		}
	}
	t.Logf("compared %d paths against Stow::Util::parent", len(paths))
}

// parentOracle runs every path through the real Perl in one process. A path is
// sent per line; paths here never contain a newline, and Perl's chomp removes
// exactly the delimiter.
func parentOracle(t *testing.T, lib string, paths []string) []string {
	t.Helper()

	args := []string{}
	if lib != "" {
		args = append(args, "-I"+lib)
	}
	args = append(args, "-MStow::Util", "-e",
		`while (defined(my $l = <STDIN>)) { chomp $l; print Stow::Util::parent($l), "\n"; }`)

	cmd := exec.Command("perl", args...)
	cmd.Stdin = strings.NewReader(strings.Join(paths, "\n") + "\n")
	out, err := cmd.Output()
	if err != nil {
		t.Fatalf("running the Stow::Util::parent oracle: %v", err)
	}

	got := strings.Split(strings.TrimSuffix(string(out), "\n"), "\n")
	if len(got) != len(paths) {
		t.Fatalf("oracle returned %d answers, want %d", len(got), len(paths))
	}
	return got
}
