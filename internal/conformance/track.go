package conformance

import (
	"os"
	"os/exec"
	"regexp"
	"strings"
	"testing"
)

// TrackOracleInput teaches `go test`'s result cache about a file that only a
// subprocess reads.
//
// The cache decides a stored result is still valid by hashing the test binary,
// its arguments, the environment variables the test read, and the files the test
// opened or stat'd *through package os*. Nothing a subprocess touches is visible
// to it — and every oracle in this repository lives in a subprocess: real stow,
// real perl, Stow.pm, Getopt::Long, and the two .pl driver scripts.
//
// This was measured, not theorised. Replacing Stow.pm's ignore() with
// `return 0` and re-running `go test -tags oracle ./stow/` printed
//
//	ok  github.com/rocne/gostow/stow  (cached)
//
// — 1216 ignore verdicts "verified" against an oracle that answers no to
// everything. The same held for both .pl scripts. The stow *binary* escaped only
// by luck: exec.Command stats it through LookPath, which the cache does record.
//
// Reading the file here puts an `open` entry in the test log, and its content
// hash becomes part of the cache key. Edit the oracle, and the test re-runs.
//
// This makes caching *correct* rather than merely disabled. CI additionally
// passes -count=1, because a cache that is correct by construction is still
// worth not relying on when the result gates a release.
func TrackOracleInput(t *testing.T, paths ...string) {
	t.Helper()
	for _, p := range paths {
		if _, err := os.ReadFile(p); err != nil {
			t.Fatalf("oracle input %s is unreadable: %v", p, err)
		}
	}
}

// PerlModulePath asks perl where it actually loaded a module from, so that module
// can be handed to TrackOracleInput. libDir is prepended to @INC when non-empty.
//
// Guessing the path is what let a stale Stow.pm hide: the module that matters is
// the one perl resolves, not the one we expect it to.
func PerlModulePath(t *testing.T, libDir, module string) string {
	t.Helper()

	key := strings.ReplaceAll(module, "::", "/") + ".pm"
	args := []string{}
	if libDir != "" {
		args = append(args, "-I"+libDir)
	}
	args = append(args, "-M"+module, "-e", `print $INC{"`+key+`"}`)

	out, err := exec.Command("perl", args...).Output()
	if err != nil {
		t.Fatalf("perl cannot load %s (lib=%q): %v", module, libDir, err)
	}
	path := strings.TrimSpace(string(out))
	if path == "" {
		t.Fatalf("perl loaded %s but reported no path for it", module)
	}
	return path
}

// RequirePerl fails when perl is missing.
//
// It used to skip. Under `-tags oracle` the caller has explicitly asked to be
// compared against a Perl program; perl's absence is a broken installation, and
// a conformance test that silently skips is a vacuous pass.
func RequirePerl(t *testing.T) {
	t.Helper()
	if _, err := exec.LookPath("perl"); err != nil {
		t.Fatalf("perl not found, but the `oracle` build tag requires it: %v", err)
	}
}

// MentionsFlag reports whether help names flag as a whole token, so "--no" is not
// satisfied by "--no-folding" and "-d" is not satisfied by "-dir".
func MentionsFlag(help, flag string) bool {
	return regexp.MustCompile(regexp.QuoteMeta(flag) + `([^\w-]|$)`).MatchString(help)
}
