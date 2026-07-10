//go:build oracle

package stow

import (
	"os/exec"
	"strconv"
	"strings"
	"testing"

	"github.com/rocne/gostow/internal/conformance"
)

// joinPaths, perlCanonpath and removeParentRefs are transcriptions of Perl, and a
// transcription pinned only by hand-written expectations is exactly how the
// `parent` bug was born: two Go ports of one Perl routine, agreeing with each
// other's author and not with Perl. paths_test.go's cases are a hand-port of
// stow's t/join_paths.t, which means they test what stow's authors thought to
// test — not what Perl does.
//
// So ask Perl. Every path here goes through the real Stow::Util::join_paths.
// gostow writes the link destinations this function returns; if it is wrong, the
// symlinks point somewhere else.
func TestJoinPathsAgreesWithStowUtil(t *testing.T) {
	lib := conformance.OraclePerlLib(t, conformance.OraclePath(t))
	conformance.TrackOracleInput(t, conformance.PerlModulePath(t, lib, "Stow::Util"))

	// Each case is one join_paths call: the fields are its arguments. The wire
	// format is "<argc>\t<arg>\t<arg>...", because an empty line cannot otherwise
	// distinguish join_paths() from join_paths("") — and those are different
	// calls, even though Perl happens to answer "" to both.
	cases := [][]string{
		{}, {""}, {"", ""}, {"a"}, {"a", "b"}, {"a", "b", "c"},
		{"", "a"}, {"a", ""}, {"", "a", ""},
		{"/", "a"}, {"/a", "b"}, {"a", "/b"}, {"/", "/"},
		{"a/", "b"}, {"a", "/b/"}, {"a/", "/b/"},
		{"a//", "//b"}, {"a///b", "c"},
		{"..", "a"}, {"a", ".."}, {"a", "..", "b"}, {"a/b", "../c"},
		{"../..", "a"}, {"a", "../.."}, {"a/b/c", "../../d"},
		{"..", ".."}, {"/..", "a"}, {"/../..", "a"},
		{".", "a"}, {"a", "."}, {"./a", "b"}, {"a", "./b"}, {"a/./b", "c"},
		{"./", "a"}, {".", "."}, {"./.", "./."},

		// The link destinations stow actually builds: "../" x level, then the
		// package path from the cwd.
		{"../", "stow/pkg/f"}, {"../../", "stow/pkg/sub/f"},
		{"../../../", "../other/pkg/f"},
		{"..", "stow", "pkg", "f"},

		// Perl falsiness bait. join_paths guards with `length $part`, not with
		// truthiness, so a "0" fragment survives. A port that wrote `if part != ""`
		// as `if part` in some other language would drop it.
		{"0"}, {"0", "a"}, {"a", "0"}, {"0", "0"}, {"a", "0", "b"},
		{"00"}, {"0.0"}, {"0", ""},

		// Whitespace and characters a path may legally contain.
		{" ", "a"}, {"a b", "c d"}, {"-", "a"}, {"a", "-"},
		{"a\\b", "c"}, {"a*b", "c"}, {"a(b", "c"},
	}

	want := joinPathsOracle(t, lib, cases)
	for i, c := range cases {
		if got := joinPaths(c...); got != want[i] {
			t.Errorf("joinPaths(%q) = %q, Stow::Util::join_paths says %q", c, got, want[i])
		}
	}
	t.Logf("compared %d join_paths calls against Stow::Util::join_paths", len(cases))
}

// joinPathsOracle runs every case through one Perl process, one line per call.
func joinPathsOracle(t *testing.T, lib string, cases [][]string) []string {
	t.Helper()

	var stdin strings.Builder
	for _, c := range cases {
		for _, arg := range c {
			if strings.ContainsAny(arg, "\t\n") {
				t.Fatalf("argument %q contains the wire's own delimiter", arg)
			}
		}
		stdin.WriteString(strconv.Itoa(len(c)))
		for _, arg := range c {
			stdin.WriteString("\t" + arg)
		}
		stdin.WriteString("\n")
	}

	args := []string{}
	if lib != "" {
		args = append(args, "-I"+lib)
	}
	args = append(args, "-MStow::Util", "-e",
		`while (defined(my $l = <STDIN>)) { chomp $l;`+
			` my @f = split(/\t/, $l, -1); my $n = shift @f;`+
			` my @p = $n ? @f[0 .. $n - 1] : ();`+
			` print Stow::Util::join_paths(@p), "\n"; }`)

	cmd := exec.Command("perl", args...)
	cmd.Stdin = strings.NewReader(stdin.String())
	out, err := cmd.Output()
	if err != nil {
		t.Fatalf("running the Stow::Util::join_paths oracle: %v", err)
	}

	got := strings.Split(strings.TrimSuffix(string(out), "\n"), "\n")
	if len(got) != len(cases) {
		t.Fatalf("oracle returned %d answers, want %d", len(got), len(cases))
	}
	return got
}
