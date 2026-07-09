package conformance

import (
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"sync"
	"testing"
)

var (
	gostowOnce sync.Once
	gostowBin  string
	gostowDir  string
	gostowErr  error
)

// GostowPath builds ./cmd/gostow once per test binary run and returns the path.
// The binary is named "stow", not "gostow": real stow derives its program name
// from basename($0) and prints it in the synopsis and in usage-error prefixes,
// so naming ours "stow" makes those bytes comparable in the differential suite.
func GostowPath(t *testing.T) string {
	t.Helper()
	gostowOnce.Do(func() {
		repo, err := repoRoot()
		if err != nil {
			gostowErr = err
			return
		}
		dir, err := os.MkdirTemp("", "gostow-bin-")
		if err != nil {
			gostowErr = err
			return
		}
		gostowDir = dir

		bin := filepath.Join(dir, "stow")
		cmd := exec.Command("go", "build", "-o", bin, "./cmd/gostow")
		cmd.Dir = repo
		if out, err := cmd.CombinedOutput(); err != nil {
			gostowErr = errors.New("building gostow: " + err.Error() + "\n" + string(out))
			return
		}
		gostowBin = bin
	})
	if gostowErr != nil {
		t.Fatalf("%v", gostowErr)
	}
	return gostowBin
}

// CleanupGostowBuild removes the shared build directory. It must be called from
// TestMain, never from a t.Cleanup: the binary is built once and shared by every
// test in the package, so tying its lifetime to whichever test happened to ask
// for it first would delete it out from under all the others.
func CleanupGostowBuild() {
	if gostowDir != "" {
		_ = os.RemoveAll(gostowDir)
	}
}

// repoRoot walks up from the working directory to the go.mod at the module root.
func repoRoot() (string, error) {
	dir, err := os.Getwd()
	if err != nil {
		return "", err
	}
	for {
		if _, err := os.Stat(filepath.Join(dir, "go.mod")); err == nil {
			return dir, nil
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			return "", errors.New("go.mod not found above working directory")
		}
		dir = parent
	}
}
