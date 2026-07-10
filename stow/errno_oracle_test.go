//go:build oracle

package stow

import (
	"bufio"
	"os/exec"
	"strconv"
	"strings"
	"syscall"
	"testing"

	"github.com/rocne/gostow/internal/conformance"
)

// stow writes "$!" into its fatal messages, so the referent for errnoText is not
// a table someone typed out — it is Perl's own stringification of errno on this
// machine, with this C library. Ask it.
//
// The bug this pins: gostow formatted the Go error with %v, which prints
// "open /abs/path: permission denied" where stow prints "Permission denied". A
// hand-written expectation would have encoded whatever the author believed about
// Go's error strings; the author believed they were already capitalised, and they
// are not.
func TestErrnoTextMatchesPerlStrerror(t *testing.T) {
	conformance.RequirePerl(t)

	// Every errno stow can plausibly surface from the syscalls it makes, plus the
	// ones the audit named: EACCES on an unreadable package directory, EISDIR on a
	// .stowrc that is a directory, EXDEV on --adopt across a mount point, ENOSPC
	// on a full disk.
	errnos := []syscall.Errno{
		syscall.EPERM, syscall.ENOENT, syscall.EIO, syscall.EBADF, syscall.EACCES,
		syscall.EFAULT, syscall.EBUSY, syscall.EEXIST, syscall.EXDEV, syscall.ENODEV,
		syscall.ENOTDIR, syscall.EISDIR, syscall.EINVAL, syscall.EMFILE, syscall.ENOTTY,
		syscall.EFBIG, syscall.ENOSPC, syscall.EROFS, syscall.EMLINK, syscall.ENAMETOOLONG,
		syscall.ENOTEMPTY, syscall.ELOOP,
	}

	want := perlStrerror(t, errnos)
	for _, e := range errnos {
		got := errnoText(e)
		if got != want[int(e)] {
			t.Errorf("errno %d: errnoText = %q, Perl's $! = %q", int(e), got, want[int(e)])
		}
	}
	t.Logf("compared %d errno strings against Perl's $!", len(errnos))
}

// perlStrerror asks Perl to stringify each errno, exactly as `$!` does when stow
// interpolates it. One subprocess, one line per errno: "<n> <string>".
func perlStrerror(t *testing.T, errnos []syscall.Errno) map[int]string {
	t.Helper()

	nums := make([]string, len(errnos))
	for i, e := range errnos {
		nums[i] = strconv.Itoa(int(e))
	}
	script := `for my $n (@ARGV) { $! = $n; print "$n $!\n"; }`
	out, err := exec.Command("perl", append([]string{"-e", script}, nums...)...).Output()
	if err != nil {
		t.Fatalf("perl strerror oracle: %v", err)
	}

	got := map[int]string{}
	sc := bufio.NewScanner(strings.NewReader(string(out)))
	for sc.Scan() {
		n, s, ok := strings.Cut(sc.Text(), " ")
		if !ok {
			continue
		}
		num, err := strconv.Atoi(n)
		if err != nil {
			t.Fatalf("perl printed a non-numeric errno %q", n)
		}
		got[num] = s
	}
	if len(got) != len(errnos) {
		t.Fatalf("perl returned %d strings for %d errnos", len(got), len(errnos))
	}
	return got
}

// errnoText must not mangle an error that carries no errno at all.
func TestErrnoTextPassesThroughNonErrno(t *testing.T) {
	err := &FatalError{Msg: "not a syscall error"}
	if got := errnoText(err); got != "not a syscall error" {
		t.Errorf("errnoText(%v) = %q, want the error's own text", err, got)
	}
}
