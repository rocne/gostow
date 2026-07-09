package main

import (
	"bytes"
	"runtime/debug"
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

// resolveVersion exists because two build paths produce gostow and only one of
// them sets the ldflag.
//
// `go install github.com/rocne/gostow/cmd/gostow@v0.1.0` passes no ldflags, so
// v0.1.0 installed that way reported "gostow dev" — a lie, since the toolchain
// knew exactly which module version it fetched and had already stamped it into
// the build info. Probed against the real published module, which is how the gap
// was found.
//
// rocne/release-ci's install smoke greps `--version` for "${VERSION#v}", so both
// the stripped and unstripped spellings satisfy it; nothing here is load-bearing
// for the pipeline, only for the user reading the line.
func TestResolveVersion(t *testing.T) {
	buildInfo := func(v string) func() (*debug.BuildInfo, bool) {
		return func() (*debug.BuildInfo, bool) {
			return &debug.BuildInfo{Main: debug.Module{Version: v}}, true
		}
	}
	noBuildInfo := func() (*debug.BuildInfo, bool) { return nil, false }

	for _, tc := range []struct {
		name   string
		ldflag string
		info   func() (*debug.BuildInfo, bool)
		want   string
	}{
		{"goreleaser sets the ldflag", "v0.1.0", buildInfo("(devel)"), "v0.1.0"},
		{"ldflag beats build info", "v0.2.0", buildInfo("v0.1.0"), "v0.2.0"},
		{"go install: fall back to the module version", "dev", buildInfo("v0.1.0"), "v0.1.0"},
		{"go build from a working tree", "dev", buildInfo("(devel)"), "dev"},
		{"build info with no version", "dev", buildInfo(""), "dev"},
		{"no build info at all", "dev", noBuildInfo, "dev"},
		{"empty ldflag is not a version", "", buildInfo("v0.1.0"), "v0.1.0"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if got := resolveVersion(tc.ldflag, tc.info); got != tc.want {
				t.Errorf("resolveVersion(%q, …) = %q, want %q", tc.ldflag, got, tc.want)
			}
		})
	}
}

// The two halves compose: a module version arrives as "v0.1.0" and must reach the
// user as "gostow 0.1.0".
func TestGoInstalledBinaryReportsTheModuleVersion(t *testing.T) {
	v := resolveVersion("dev", func() (*debug.BuildInfo, bool) {
		return &debug.BuildInfo{Main: debug.Module{Version: "v0.1.0"}}, true
	})
	if got, want := cli.IdentityLine(v), "gostow 0.1.0 (GNU Stow 2.4.1 compatible)"; got != want {
		t.Errorf("identity line = %q, want %q", got, want)
	}
}
