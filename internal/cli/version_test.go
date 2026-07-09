package cli

import (
	"bytes"
	"regexp"
	"strings"
	"testing"
)

// The release pipeline injects release-please's tag_name into main.version, so
// the string the shipped binary actually holds is "v0.1.0", not "0.1.0".
//
// Every other test in this package passes a hand-written "0.1.0", which is why
// v0.1.0 shipped announcing itself as "gostow v0.1.0" while docs/SPEC.md §4.4,
// docs/DIVERGENCES.md and cli_test.go all promised "gostow 0.1.0". A test that
// only ever sees a value the pipeline never produces cannot catch a bug in what
// the pipeline produces.
//
// So this table is written from the pipeline's side: every form a version can
// arrive in.
func TestIdentityLineStripsTheTagPrefix(t *testing.T) {
	for _, tc := range []struct{ version, want string }{
		{"v0.1.0", "gostow 0.1.0 (GNU Stow 2.4.1 compatible)"}, // goreleaser, via tag_name
		{"0.1.0", "gostow 0.1.0 (GNU Stow 2.4.1 compatible)"},  // a distro packager
		{"v1.2.3-rc.1", "gostow 1.2.3-rc.1 (GNU Stow 2.4.1 compatible)"},
		{"dev", "gostow dev (GNU Stow 2.4.1 compatible)"}, // the default; not a version
		{"", "gostow  (GNU Stow 2.4.1 compatible)"},
		{"v", "gostow v (GNU Stow 2.4.1 compatible)"},           // not "v"+digit
		{"vendor", "gostow vendor (GNU Stow 2.4.1 compatible)"}, // leading v, not a version
		{"version-2", "gostow version-2 (GNU Stow 2.4.1 compatible)"},
	} {
		if got := IdentityLine(tc.version); got != tc.want {
			t.Errorf("IdentityLine(%q) = %q, want %q", tc.version, got, tc.want)
		}
	}
}

// reVersionLine is the shape docs/SPEC.md §4.4 promises. A leading "v" would
// break it.
var reVersionLine = regexp.MustCompile(`^gostow \d+\.\d+\.\d+\S* \(GNU Stow \d+\.\d+\.\d+ compatible\)$`)

// TestVersionOutputMatchesTheDocumentedShape drives the real CLI with the exact
// string the release pipeline injects, through --version and --help, and requires
// the documented line.
func TestVersionOutputMatchesTheDocumentedShape(t *testing.T) {
	const pipelineVersion = "v0.1.0" // release-please tag_name, verbatim

	for _, args := range [][]string{{"--version"}, {"--help"}} {
		var out, errb bytes.Buffer
		if code := Run(append([]string{"stow"}, args...), pipelineVersion, &out, &errb); code != 0 {
			t.Fatalf("%v: exit = %d", args, code)
		}
		line := strings.SplitN(out.String(), "\n", 2)[0]
		if !reVersionLine.MatchString(line) {
			t.Errorf("%v printed %q, which does not match the documented identity line", args, line)
		}
		if strings.Contains(line, "gostow v") {
			t.Errorf("%v: the tag's leading \"v\" reached the user: %q", args, line)
		}
	}
}
