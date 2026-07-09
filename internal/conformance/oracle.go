//go:build oracle

package conformance

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

// oracleVersion is the pinned conformance referent. A binary reporting anything
// else silently redefines the spec, so a mismatch is fatal, not a skip.
const oracleVersion = "version 2.4.1"

// OraclePath locates real stow 2.4.1: GOSTOW_ORACLE wins, then the repo's
// .oracle/bin/stow, then $PATH. Absent oracle skips; wrong-version oracle fails.
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
		t.Skip("no stow oracle: set GOSTOW_ORACLE, build .oracle/bin/stow, or install stow")
	}

	out, err := exec.Command(bin, "--version").CombinedOutput()
	if err != nil {
		t.Fatalf("oracle %q --version failed: %v", bin, err)
	}
	if !strings.Contains(string(out), oracleVersion) {
		t.Fatalf("oracle %q is not %s: %q", bin, oracleVersion, strings.TrimSpace(string(out)))
	}
	return bin
}
