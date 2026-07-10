//go:build oracle

package conformance

import (
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"testing"
)

// GNU Stow describes its options in two places, and neither is complete:
//
//   - `stow --help` never mentions --no-folding (ledger PL-16).
//   - stow.8 never mentions --compat, nor its short form -p.
//
// A user who reads one and not the other comes away believing gostow is missing
// an option. So the claim this file pins is about the union: **every option that
// GNU Stow documents anywhere, gostow's man page documents.**
//
// The hermetic half lives in internal/cli/docs_test.go, which requires
// man/gostow.8 to name exactly the options spec() accepts. Composing the two:
// stow's help ∪ stow's manual ⊆ gostow's man page = gostow's parser. The oracle
// half cannot be hermetic — stow.8 is installed by the oracle, not checked in.
func TestManPageDocumentsEveryOptionStowDocumentsAnywhere(t *testing.T) {
	oracle := OraclePath(t)
	stowMan := oracleManPage(t, oracle)

	// stow.8 is a subprocess input in spirit — it is read from the oracle's
	// install tree, not through this test's own package os calls until now — and
	// the rendered text depends on the installed groff. Track both files so a
	// reinstalled oracle invalidates the cached result.
	TrackOracleInput(t, stowMan)

	fromHelp := documentedFlags(mustHelp(t, "oracle", oracle))
	fromMan := documentedFlags(renderManPage(t, stowMan))

	if len(fromHelp) < 10 || len(fromMan) < 10 {
		t.Fatalf("extracted %d flags from stow --help and %d from stow.8; a parse is broken "+
			"and this test would be vacuous", len(fromHelp), len(fromMan))
	}

	// The premise of this whole test: the two references really do disagree.
	// If a future stow release fixes that, this assertion is how we find out.
	assertAsymmetry(t, "stow.8", fromMan, "stow --help", fromHelp)

	want := union(fromHelp, fromMan)
	t.Logf("GNU Stow documents %d options across both references (%d in --help, %d in stow.8): %s",
		len(want), len(fromHelp), len(fromMan), strings.Join(want, " "))

	gostowMan := readGostowManPage(t)
	for _, flag := range want {
		if !MentionsFlag(gostowMan, flag) {
			t.Errorf("GNU Stow documents %s; gostow's man page never names it", flag)
		}
	}
}

// assertAsymmetry states, and therefore verifies, that each reference omits an
// option the other names. It is the reason this test reads both.
func assertAsymmetry(t *testing.T, aName string, a []string, bName string, b []string) {
	t.Helper()
	onlyA := difference(a, b)
	onlyB := difference(b, a)
	if len(onlyA) == 0 && len(onlyB) == 0 {
		t.Errorf("%s and %s now document the same options. That is good news upstream, "+
			"but this test exists because they did not — re-read it before deleting it.", aName, bName)
		return
	}
	t.Logf("%s alone documents %v; %s alone documents %v", aName, onlyA, bName, onlyB)
}

// oracleManPage locates stow.8 in the oracle's install tree, next to its bin/.
func oracleManPage(t *testing.T, oracleBin string) string {
	t.Helper()
	prefix := filepath.Dir(filepath.Dir(oracleBin))
	path := filepath.Join(prefix, "share", "man", "man8", "stow.8")
	if _, err := os.Stat(path); err != nil {
		t.Fatalf("the oracle's man page is not at %s (%v). install-stow-oracle.sh should have "+
			"installed it; without it this test would be vacuous", path, err)
	}
	return path
}

// renderManPage turns roff into the text a reader sees. groff is required: doing
// it by hand would be a second, unverified implementation of the thing under test.
func renderManPage(t *testing.T, path string) string {
	t.Helper()
	groff, err := exec.LookPath("groff")
	if err != nil {
		t.Fatalf("groff is not installed, so stow.8 cannot be rendered and this test "+
			"cannot run: %v", err)
	}
	out, err := exec.Command(groff, "-man", "-Tutf8", "-P", "-c", path).Output()
	if err != nil {
		t.Fatalf("rendering %s: %v", path, err)
	}
	// -Tutf8 sets overstrike for bold; strip the backspace sequences col(1) would.
	return regexp.MustCompile(".\x08").ReplaceAllString(string(out), "")
}

// readGostowManPage returns the page's *prose*, with roff comments removed.
//
// Dropping the comments is load bearing. The header comment explains why this
// file exists, and in doing so it names --compat and --no-folding — the two
// options stow's own references omit. Feeding it to MentionsFlag made the test
// pass on a page that documented neither: deleting --compat from the OPTIONS
// section changed nothing, because the comment still mentioned it. Caught by
// mutating the page and watching the test stay green.
func readGostowManPage(t *testing.T) string {
	t.Helper()
	root, err := repoRoot()
	if err != nil {
		t.Fatal(err)
	}
	b, err := os.ReadFile(filepath.Join(root, "man", "gostow.8"))
	if err != nil {
		t.Fatal(err)
	}
	var lines []string
	for _, line := range strings.Split(string(b), "\n") {
		if strings.HasPrefix(line, `.\"`) {
			continue
		}
		lines = append(lines, line)
	}
	// "\-" is the only escape that hides an option name from MentionsFlag.
	return strings.ReplaceAll(strings.Join(lines, "\n"), `\-`, "-")
}

func union(a, b []string) []string {
	seen := map[string]bool{}
	for _, x := range append(append([]string{}, a...), b...) {
		seen[x] = true
	}
	out := make([]string, 0, len(seen))
	for x := range seen {
		out = append(out, x)
	}
	sort.Strings(out)
	return out
}

func difference(a, b []string) []string {
	in := map[string]bool{}
	for _, x := range b {
		in[x] = true
	}
	var out []string
	for _, x := range a {
		if !in[x] {
			out = append(out, x)
		}
	}
	sort.Strings(out)
	return out
}
