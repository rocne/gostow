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
		t.Cleanup(func() { _ = os.RemoveAll(dir) })

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
