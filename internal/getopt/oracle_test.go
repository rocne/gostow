//go:build oracle

package getopt

import (
	"fmt"
	"math/rand"
	"os/exec"
	"sort"
	"strconv"
	"strings"
	"testing"
)

// The hand-written table in getopt_test.go is a transcription of probe output,
// and a transcription can be wrong in exactly the places that matter. This test
// removes the transcription: it drives real Getopt::Long, configured exactly as
// stow configures it, over a few thousand generated argv vectors and requires
// gostow's parser to agree on every one.
//
// The comparison is on *parse state* -- the options hash, the two package lists,
// the leftover array and the diagnostics -- because that is precisely what
// bin/stow consumes from GetOptionsFromArray. Event order is an implementation
// detail; the fold below is the same one internal/cli performs.

// foldPerlDump replays Events into the state bin/stow would hold, and renders it
// exactly as testdata/getopt_oracle.pl renders Perl's.
func foldPerlDump(r Result) string {
	flags := map[string]string{}
	lists := map[string][]string{}
	var verbose *int
	action := "stow"
	var unstow, stowed []string

	for _, e := range r.Events {
		switch e.Option {
		case "":
			switch action {
			case "restow":
				unstow = append(unstow, e.Arg)
				stowed = append(stowed, e.Arg)
			case "unstow":
				unstow = append(unstow, e.Arg)
			default:
				stowed = append(stowed, e.Arg)
			}
		case "D":
			action = "unstow"
		case "S":
			action = "stow"
		case "R":
			action = "restow"
		case "verbose":
			if e.HasValue {
				n, err := strconv.Atoi(e.Value)
				if err != nil {
					panic("parser admitted a non-integer verbose value: " + e.Value)
				}
				verbose = &n
				continue
			}
			if verbose == nil {
				zero := 0
				verbose = &zero
			}
			*verbose++
		case "ignore", "override", "defer":
			lists[e.Option] = append(lists[e.Option], e.Value)
		case "dir", "target":
			flags[e.Option] = e.Value
		default:
			flags[e.Option] = "1"
		}
	}

	if verbose != nil {
		flags["verbose"] = strconv.Itoa(*verbose)
	}
	for k, v := range lists {
		flags[k] = strings.Join(v, ",")
	}

	var b strings.Builder
	fmt.Fprintf(&b, "ok=%d\n", boolToInt(r.OK()))
	keys := make([]string, 0, len(flags))
	for k := range flags {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	for _, k := range keys {
		fmt.Fprintf(&b, "opt %s=%s\n", k, flags[k])
	}
	fmt.Fprintf(&b, "unstow=%s\n", strings.Join(unstow, ","))
	fmt.Fprintf(&b, "stow=%s\n", strings.Join(stowed, ","))
	fmt.Fprintf(&b, "leftover=%s\n", strings.Join(r.Leftover, ","))
	for _, w := range r.Errors {
		fmt.Fprintf(&b, "warn=%s\n", w)
	}
	return b.String()
}

func boolToInt(b bool) int {
	if b {
		return 1
	}
	return 0
}

// corpusTokens are chosen to exercise every branch of the parser: bundling,
// abbreviation, exact-alias-beats-prefix, the ':+' integer rules, missing and
// empty values, the "--" terminator, and each diagnostic.
var corpusTokens = []string{
	"-v", "-vv", "-v3", "-v0", "-v-1", "-v=3", "-vabc",
	"--verbose", "--verbose=2", "--verbose=", "--verbose=abc", "--verbose=+2",
	"-n", "--no", "--nofolding", "--no-f", "--no-",
	"--dotfiles", "--dot", "--dotfiles=1",
	"-d", "-ddir", "--dir=x", "--dir=", "--d", "--dir",
	"-t", "-ttgt", "--target=y", "--targ", "--tar",
	"-D", "-S", "-R", "--delete", "--restow", "--st", "--s",
	"--ignore=a", "--defer=b", "--override=c", "--ignore=",
	"-p", "-h", "-V", "--adopt", "--a", "--de", "--ver",
	"-x", "-", "--", "pkg", "pkg2", "stow", "tgt", "3",
}

func buildCorpus() [][]string {
	var corpus [][]string
	corpus = append(corpus, nil) // the empty argv
	for _, a := range corpusTokens {
		corpus = append(corpus, []string{a})
	}
	for _, a := range corpusTokens {
		for _, b := range corpusTokens {
			corpus = append(corpus, []string{a, b})
		}
	}
	// Deterministic sample of longer vectors: exhaustive triples would be
	// ~175k, which buys little over a fixed-seed sample.
	rng := rand.New(rand.NewSource(20240908)) // stow 2.4.1's release date
	for i := 0; i < 3000; i++ {
		n := 3 + rng.Intn(3)
		v := make([]string, n)
		for j := range v {
			v[j] = corpusTokens[rng.Intn(len(corpusTokens))]
		}
		corpus = append(corpus, v)
	}
	return corpus
}

func TestParseAgreesWithRealGetoptLong(t *testing.T) {
	if _, err := exec.LookPath("perl"); err != nil {
		t.Skip("perl not available; the oracle build tag implies it should be")
	}

	corpus := buildCorpus()

	var stdin strings.Builder
	for _, v := range corpus {
		stdin.WriteString(strings.Join(v, "\x1f"))
		stdin.WriteString("\n")
	}

	cmd := exec.Command("perl", "testdata/getopt_oracle.pl")
	cmd.Stdin = strings.NewReader(stdin.String())
	out, err := cmd.Output()
	if err != nil {
		t.Fatalf("running the Getopt::Long oracle: %v", err)
	}

	records := strings.Split(string(out), "\x1e")
	if len(records) != len(corpus)+1 { // trailing empty after the final separator
		t.Fatalf("oracle returned %d records, want %d", len(records)-1, len(corpus))
	}

	spec := stowSpec()
	mismatches := 0
	for i, v := range corpus {
		want := records[i]
		got := foldPerlDump(Parse(spec, v))
		if got != want {
			mismatches++
			if mismatches <= 10 {
				t.Errorf("argv %q\n--- gostow ---\n%s--- Getopt::Long ---\n%s", v, got, want)
			}
		}
	}
	if mismatches > 10 {
		t.Errorf("... and %d further mismatches", mismatches-10)
	}
	t.Logf("compared %d argv vectors against real Getopt::Long", len(corpus))
}
