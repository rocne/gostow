package stow

import (
	"os"
	"path/filepath"
	"runtime"
	"syscall"
	"testing"
	"time"
)

// crossDeviceDir returns a directory on a different filesystem from ref, or stops
// the test.
//
// No mounting and no root required: /dev/shm is a tmpfs on every ordinary Linux,
// and it is not the filesystem holding /tmp. Set GOSTOW_TEST_XDEV_DIR to point
// somewhere else — a second mount in CI, say — and it is used instead.
//
// The device numbers are compared rather than assumed. A test that means to cross
// a filesystem boundary and quietly does not is the vacuous pass this project
// keeps finding; here it would leave `os.Rename` succeeding and the whole fallback
// unexercised, reporting PASS.
func crossDeviceDir(t *testing.T, ref string) string {
	t.Helper()

	base := os.Getenv("GOSTOW_TEST_XDEV_DIR")
	if base == "" {
		if runtime.GOOS != "linux" {
			t.Skip("no second filesystem is guaranteed off linux; set GOSTOW_TEST_XDEV_DIR. " +
				"CI's linux jobs cover this, and moveFile is platform-independent Go.")
		}
		base = "/dev/shm"
	}
	if fi, err := os.Stat(base); err != nil || !fi.IsDir() {
		t.Fatalf("%s is not a usable directory (%v); set GOSTOW_TEST_XDEV_DIR", base, err)
	}

	dir, err := os.MkdirTemp(base, "gostow-xdev-")
	if err != nil {
		t.Fatalf("creating a directory on %s: %v", base, err)
	}
	t.Cleanup(func() { _ = os.RemoveAll(dir) })

	if device(t, dir) == device(t, ref) {
		t.Fatalf("%s and %s are on the same filesystem, so this test would not cross one", dir, ref)
	}
	return dir
}

func device(t *testing.T, path string) uint64 {
	t.Helper()
	var st syscall.Stat_t
	if err := syscall.Stat(path, &st); err != nil {
		t.Fatalf("stat %s: %v", path, err)
	}
	return uint64(st.Dev) //nolint:unconvert,nolintlint // Dev is int32 on darwin, uint64 on linux
}

// Stow.pm calls File::Copy::move and says why: "rename() not good enough, since
// the stow directory might be on a different filesystem to the target." gostow
// called os.Rename, so `--adopt` with ~/dotfiles on one volume and $HOME on
// another printed MV: and LINK:, then aborted with EXDEV having changed nothing.
func TestAdoptAcrossAFilesystemBoundary(t *testing.T) {
	dir := t.TempDir()
	target := crossDeviceDir(t, dir)

	write(t, filepath.Join(dir, "pkg", "f"), "package version")
	write(t, filepath.Join(target, "f"), "target version")

	if _, err := Stow(Options{Dir: dir, Target: target, Fold: true, Adopt: true}, "pkg"); err != nil {
		t.Fatalf("Stow --adopt across filesystems: %v", err)
	}

	got, err := os.ReadFile(filepath.Join(dir, "pkg", "f"))
	if err != nil {
		t.Fatal(err)
	}
	if string(got) != "target version" {
		t.Errorf("package file = %q, want the adopted target's content", got)
	}
	fi, err := os.Lstat(filepath.Join(target, "f"))
	if err != nil {
		t.Fatal(err)
	}
	if fi.Mode()&os.ModeSymlink == 0 {
		t.Error("target/f is not a symlink after a cross-filesystem --adopt")
	}
}

// The fallback's observable effects, each probed against Perl 5.40's File::Copy
// before being written down: the destination keeps its own mode when it exists,
// the times come from the source, and the source is gone.
func TestMoveFileFallbackSemantics(t *testing.T) {
	src := t.TempDir()
	dst := crossDeviceDir(t, src)

	srcPath := filepath.Join(src, "src")
	dstPath := filepath.Join(dst, "dst")
	write(t, srcPath, "source contents")
	if err := os.Chmod(srcPath, 0o755); err != nil {
		t.Fatal(err)
	}
	mtime := time.Date(2019, 5, 6, 7, 8, 9, 0, time.UTC)
	if err := os.Chtimes(srcPath, mtime, mtime); err != nil {
		t.Fatal(err)
	}

	// A destination that already exists — which is every --adopt, since the
	// destination is the package's own file.
	write(t, dstPath, "old contents that must be truncated")
	if err := os.Chmod(dstPath, 0o600); err != nil {
		t.Fatal(err)
	}

	if err := moveFile(srcPath, dstPath); err != nil {
		t.Fatalf("moveFile across filesystems: %v", err)
	}

	if _, err := os.Lstat(srcPath); !os.IsNotExist(err) {
		t.Errorf("source survived the move (err = %v)", err)
	}
	got, err := os.ReadFile(dstPath)
	if err != nil {
		t.Fatal(err)
	}
	if string(got) != "source contents" {
		t.Errorf("destination content = %q, want the source's", got)
	}
	fi, err := os.Stat(dstPath)
	if err != nil {
		t.Fatal(err)
	}
	if fi.Mode().Perm() != 0o600 {
		t.Errorf("destination mode = %o, want 0600 — copy() must not carry the source's mode", fi.Mode().Perm())
	}
	if !fi.ModTime().Equal(mtime) {
		t.Errorf("destination mtime = %v, want the source's %v", fi.ModTime(), mtime)
	}
}

// A same-filesystem move is a plain rename, which carries the source's mode. That
// the two cases disagree about mode is not a bug in either: it is what
// File::Copy::move does, and stow does it too.
func TestMoveFileOnOneFilesystemIsARenameAndCarriesTheMode(t *testing.T) {
	dir := t.TempDir()
	src := filepath.Join(dir, "src")
	dst := filepath.Join(dir, "dst")
	write(t, src, "x")
	if err := os.Chmod(src, 0o755); err != nil {
		t.Fatal(err)
	}
	write(t, dst, "y")
	if err := os.Chmod(dst, 0o600); err != nil {
		t.Fatal(err)
	}

	if err := moveFile(src, dst); err != nil {
		t.Fatalf("moveFile: %v", err)
	}
	fi, err := os.Stat(dst)
	if err != nil {
		t.Fatal(err)
	}
	if fi.Mode().Perm() != 0o755 {
		t.Errorf("destination mode = %o, want 0755 — rename carries the source's mode", fi.Mode().Perm())
	}
}

// A move that cannot succeed must still report an error rather than lose the file.
func TestMoveFileReportsAFailedMove(t *testing.T) {
	dir := t.TempDir()
	src := filepath.Join(dir, "src")
	write(t, src, "x")

	if err := moveFile(src, filepath.Join(dir, "nosuchdir", "dst")); err == nil {
		t.Fatal("moveFile into a non-existent directory returned no error")
	}
	if _, err := os.Stat(src); err != nil {
		t.Errorf("the source was destroyed by a failed move: %v", err)
	}
}
