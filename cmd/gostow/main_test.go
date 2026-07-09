package main

import (
	"bytes"
	"strings"
	"testing"

	"github.com/rocne/gostow/internal/cli"
)

// The version line is a release-pipeline contract, not just cosmetics: the
// install smoke test in rocne/release-ci greps it for the released tag
// (`gostow --version | grep -F "$VER"`). It is also gostow's one intentional
// divergence from GNU stow's output — see docs/SPEC.md §4.4 and ledger PL-12.
func TestVersionLineReportsGostowVersionNotStowVersion(t *testing.T) {
	got := cli.IdentityLine("0.1.0")
	want := "gostow 0.1.0 (GNU Stow 2.4.1 compatible)"
	if got != want {
		t.Errorf("IdentityLine(%q) = %q, want %q", "0.1.0", got, want)
	}
}

// The identity line ignores basename($0) even though everything else follows it
// (ledger PL-17): it names the tool, not the invocation.
func TestVersionFlagIgnoresProgramName(t *testing.T) {
	var stdout, stderr bytes.Buffer
	if code := cli.Run([]string{"/usr/local/bin/stow", "--version"}, "0.1.0", &stdout, &stderr); code != 0 {
		t.Fatalf("exit = %d, want 0", code)
	}
	if got := strings.TrimSpace(stdout.String()); got != "gostow 0.1.0 (GNU Stow 2.4.1 compatible)" {
		t.Errorf("stdout = %q", got)
	}
}
