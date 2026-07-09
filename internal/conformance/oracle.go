//go:build oracle

package conformance

import (
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
)

// oracleVersion is the pinned conformance referent. A binary reporting anything
// else silently redefines the spec, so a mismatch is fatal, not a skip.
const oracleVersion = "version 2.4.1"

// oracleModuleVersion is the same pin, asserted against the Perl module the
// binary actually loads. `stow` and `Stow.pm` are separate files and can drift
// apart on a machine with more than one stow installed.
const oracleModuleVersion = "2.4.1"

// OraclePath locates real stow 2.4.1: GOSTOW_ORACLE wins, then the repo's
// .oracle/bin/stow, then $PATH.
//
// Every failure here is fatal. It used to skip when no oracle was found, which
// meant `go test -tags oracle ./...` on a machine without stow printed "ok" for
// every package while comparing nothing at all — the whole differential suite,
// silently absent. The build tag is the caller asking for the oracle; not having
// one is a broken installation, not a reason to pass.
//
// The binary and the module it loads are both registered with the test cache
// (see TrackOracleInput), because otherwise editing either leaves a stale PASS in
// place.
func OraclePath(t *testing.T) string {
	t.Helper()

	bin := os.Getenv("GOSTOW_ORACLE")
	if bin == "" {
		if repo, err := repoRoot(); err == nil {
			candidate := filepath.Join(repo, ".oracle", "bin", "stow")
			if _, err := os.Stat(candidate); err == nil {
				bin = candidate
			}
		}
	}
	if bin == "" {
		if found, err := exec.LookPath("stow"); err == nil {
			bin = found
		}
	}
	if bin == "" {
		t.Fatal("no stow oracle found, but the `oracle` build tag requires one.\n" +
			"Install it with:  PREFIX=$PWD/.oracle bash test/install-stow-oracle.sh\n" +
			"then:             PATH=$PWD/.oracle/bin:$PATH go test -tags oracle ./...\n" +
			"Or point GOSTOW_ORACLE at a stow " + oracleModuleVersion + " binary.\n" +
			"Do NOT `apt install stow`: Ubuntu 24.04 ships 2.3.1 and would redefine the spec.")
	}

	out, err := exec.Command(bin, "--version").CombinedOutput()
	if err != nil {
		t.Fatalf("oracle %q --version failed: %v", bin, err)
	}
	if !strings.Contains(string(out), oracleVersion) {
		t.Fatalf("oracle %q is not %s: %q", bin, oracleVersion, strings.TrimSpace(string(out)))
	}

	// stow is a perl script, so its own bytes are trackable; Stow.pm is loaded by
	// the interpreter and would otherwise be invisible to the cache.
	TrackOracleInput(t, bin, OracleStowPm(t, bin))
	return bin
}

// reUseLib captures the module directory stow's build baked into its script. The
// line is absent when the install prefix is already on perl's @INC, so its
// absence is normal, not an error.
var reUseLib = regexp.MustCompile(`(?m)^use lib "([^"]+)";`)

// OraclePerlLib returns the directory that must be prepended to @INC to load the
// *pinned* Stow.pm, or "" when perl already finds it.
//
// Guessing the layout is what once made the ignore suite skip silently in CI
// while passing locally: 2.4.1 built against perl 5.40 lands in share/perl5/5.40,
// and a /usr/local install may need no `use lib` at all. So the oracle's own
// script is read for the answer, and then perl is asked to prove it can load
// Stow 2.4.1.
func OraclePerlLib(t *testing.T, oracleBin string) string {
	t.Helper()
	RequirePerl(t)

	script, err := os.ReadFile(oracleBin)
	if err != nil {
		t.Fatalf("reading the oracle script %s: %v", oracleBin, err)
	}
	dir := ""
	if m := reUseLib.FindSubmatch(script); m != nil {
		dir = string(m[1])
	}

	args := []string{}
	if dir != "" {
		args = append(args, "-I"+dir)
	}
	args = append(args, "-MStow", "-e", "print $Stow::VERSION")
	out, err := exec.Command("perl", args...).Output()
	if err != nil {
		t.Fatalf("perl cannot load the pinned Stow.pm (lib=%q, from %s): %v", dir, oracleBin, err)
	}
	if got := strings.TrimSpace(string(out)); got != oracleModuleVersion {
		t.Fatalf("perl loaded Stow %s, want %s: a mismatched module would redefine the spec",
			got, oracleModuleVersion)
	}
	return dir
}

// OracleStowPm returns the filesystem path of the Stow.pm that the oracle binary
// will load.
func OracleStowPm(t *testing.T, oracleBin string) string {
	t.Helper()
	return PerlModulePath(t, OraclePerlLib(t, oracleBin), "Stow")
}
