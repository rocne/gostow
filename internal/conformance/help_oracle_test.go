//go:build oracle

package conformance

import (
	"regexp"
	"sort"
	"strings"
	"testing"
)

// gostow's --help is written in gostow's own words, so it is not a byte fixture
// (SPEC §4.5). Two things are still owed to the oracle, and this is the second
// of them — the first being that option *parsing* stays byte-exact, which the
// 6307 argv vectors in internal/getopt pin against real Getopt::Long.
//
// The claim here is about the interface, not the prose: **every option GNU Stow
// documents, gostow documents too.** A user reading `stow --help` and then
// running gostow must not find a flag missing.
//
// The converse is deliberately not asserted. gostow's help is allowed to name
// more: its own --gostow-* extensions, and --no-folding, which is a real,
// working, undocumented flag in stow (ledger PL-16).
func TestHelpDocumentsEveryOptionStowDocuments(t *testing.T) {
	oracle := OraclePath(t)
	gostow := GostowPath(t)

	stowHelp := mustHelp(t, "oracle", oracle)
	gostowHelp := mustHelp(t, "gostow", gostow)

	want := documentedFlags(stowHelp)
	if len(want) < 10 {
		t.Fatalf("only extracted %v from stow's help; the parse is broken and this test would be vacuous", want)
	}
	t.Logf("GNU Stow documents %d options: %s", len(want), strings.Join(want, " "))

	for _, flag := range want {
		if !MentionsFlag(gostowHelp, flag) {
			t.Errorf("stow --help documents %s; gostow --help never names it", flag)
		}
	}
}

func mustHelp(t *testing.T, who, bin string) string {
	t.Helper()
	run := RunBinary(bin, []string{"--help"}, []string{"PATH=/usr/local/bin:/usr/bin:/bin"}, t.TempDir())
	if run.ExitCode != 0 {
		t.Fatalf("%s --help exited %d: %s", who, run.ExitCode, run.Stderr)
	}
	if strings.TrimSpace(run.Stdout) == "" {
		t.Fatalf("%s --help printed nothing", who)
	}
	return run.Stdout
}

var (
	// A long option, anywhere: --dir, --verbose[=N], --no.
	reLongFlag = regexp.MustCompile(`--([a-z][a-z-]*)`)
	// A short option, only where an option list would put one: at the start of
	// an indented line, followed by a space or a comma.
	reShortFlag = regexp.MustCompile(`(?m)^\s+(-[A-Za-z])[ ,]`)
)

// documentedFlags returns the sorted, deduplicated set of options a help block
// names.
func documentedFlags(help string) []string {
	seen := map[string]bool{}
	for _, m := range reLongFlag.FindAllStringSubmatch(help, -1) {
		seen["--"+m[1]] = true
	}
	for _, m := range reShortFlag.FindAllStringSubmatch(help, -1) {
		seen[m[1]] = true
	}
	out := make([]string, 0, len(seen))
	for f := range seen {
		out = append(out, f)
	}
	sort.Strings(out)
	return out
}
