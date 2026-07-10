//go:build oracle

package stow

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/rocne/gostow/internal/conformance"
)

// The differential harness materialises one sandbox root, so no fixture it can
// express ever puts the stow directory and the target on different filesystems —
// which is exactly why nothing noticed that `--adopt` aborted there. Build the
// layout by hand and run both binaries over it.
//
// The recipe is the one a real user hits: a package directory on one volume, a
// target on another, and a file in the target that --adopt should pull in.
func TestAdoptAcrossFilesystemsAgreesWithOracle(t *testing.T) {
	oracle := conformance.OraclePath(t)

	type result struct {
		exitCode   int
		stderr     string
		pkgContent string
		pkgMode    os.FileMode
		targetLink bool
	}

	run := func(bin string) result {
		dir := t.TempDir()
		target := crossDeviceDir(t, dir)

		write(t, filepath.Join(dir, "pkg", "f"), "package version")
		if err := os.Chmod(filepath.Join(dir, "pkg", "f"), 0o600); err != nil {
			t.Fatal(err)
		}
		write(t, filepath.Join(target, "f"), "target version")
		if err := os.Chmod(filepath.Join(target, "f"), 0o755); err != nil {
			t.Fatal(err)
		}

		run := conformance.RunBinary(bin,
			[]string{"-d", dir, "-t", target, "--adopt", "-v", "pkg"},
			[]string{"HOME=" + t.TempDir(), "PATH=/usr/local/bin:/usr/bin:/bin"},
			dir)

		content, err := os.ReadFile(filepath.Join(dir, "pkg", "f"))
		if err != nil {
			t.Fatalf("%s: reading the package file: %v", bin, err)
		}
		fi, err := os.Stat(filepath.Join(dir, "pkg", "f"))
		if err != nil {
			t.Fatal(err)
		}
		li, err := os.Lstat(filepath.Join(target, "f"))
		if err != nil {
			t.Fatalf("%s: the target entry is gone: %v", bin, err)
		}

		return result{
			exitCode:   run.ExitCode,
			stderr:     conformance.Normalize(conformance.Normalize(run.Stderr, target), dir),
			pkgContent: string(content),
			pkgMode:    fi.Mode().Perm(),
			targetLink: li.Mode()&os.ModeSymlink != 0,
		}
	}

	want := run(oracle)
	got := run(conformance.GostowPath(t))

	// The oracle must actually have adopted the file, or the comparison below
	// would be satisfied by two identical failures.
	if want.exitCode != 0 || want.pkgContent != "target version" || !want.targetLink {
		t.Fatalf("the oracle did not adopt across the boundary (%+v); this test would be vacuous", want)
	}

	if got.exitCode != want.exitCode {
		t.Errorf("exit code: oracle=%d gostow=%d\ngostow stderr:\n%s", want.exitCode, got.exitCode, got.stderr)
	}
	if got.pkgContent != want.pkgContent {
		t.Errorf("package content: oracle=%q gostow=%q", want.pkgContent, got.pkgContent)
	}
	// File::Copy::copy opens the destination with O_CREAT|0666, so an existing
	// destination keeps its own mode. A rename would have carried 0755 across.
	if got.pkgMode != want.pkgMode {
		t.Errorf("package mode: oracle=%o gostow=%o", want.pkgMode, got.pkgMode)
	}
	if got.targetLink != want.targetLink {
		t.Errorf("target is a symlink: oracle=%v gostow=%v", want.targetLink, got.targetLink)
	}

	t.Logf("both adopted across a filesystem boundary; package file left at mode %o", want.pkgMode)
}
