package main

import "testing"

// The version line is a release-pipeline contract, not just cosmetics: the
// install smoke test in rocne/release-ci greps it for the released tag
// (`gostow --version | grep -F "$VER"`). It is also gostow's one intentional
// divergence from GNU stow's output — see docs/SPEC.md §4.4 and ledger PL-12.
func TestVersionLineReportsGostowVersionNotStowVersion(t *testing.T) {
	got := versionLine("0.1.0")
	want := "gostow 0.1.0 (GNU Stow 2.4.1 compatible)"
	if got != want {
		t.Errorf("versionLine(%q) = %q, want %q", "0.1.0", got, want)
	}
}

func TestVersionLineCarriesDevPlaceholder(t *testing.T) {
	got := versionLine("dev")
	want := "gostow dev (GNU Stow 2.4.1 compatible)"
	if got != want {
		t.Errorf("versionLine(%q) = %q, want %q", "dev", got, want)
	}
}
