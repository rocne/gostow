package stow

import (
	"errors"
	"io"
	"os"
	"syscall"
)

// moveFile ports Perl's File::Copy::move, which is what Stow.pm calls — and it
// calls it deliberately. `Stow.pm`, at the one site that moves a file:
//
//	# rename() not good enough, since the stow directory
//	# might be on a different filesystem to the target.
//	move $task->{path}, $task->{dest}
//
// That is not a hypothetical. `--adopt` moves a file out of the target and into
// the package, and a stow directory on a different filesystem from its target is
// an ordinary arrangement: ~/dotfiles on one volume and $HOME on another, or
// /usr/local/stow beside a separately mounted /usr/local.
//
// gostow called os.Rename, which fails with EXDEV across a mount point. Real stow
// adopts the file and exits 0; gostow aborted with
// "Could not move f -> ... (Invalid cross-device link)" after having already
// printed MV: and LINK:, leaving the target untouched and the package unchanged.
//
// File::Copy::_move's fallback is copy, then utime, then unlink — in that order,
// and it is the order that decides what survives a crash. The observable
// consequences, all probed against Perl 5.40's File::Copy:
//
//   - The destination's **mode is not the source's.** copy() opens the destination
//     with O_CREAT and mode 0666, so a new file lands at 0666 &^ umask (verified at
//     umask 000 and 077), and an existing one keeps whatever mode it already had.
//     Under --adopt the destination always exists — it is the package's own file —
//     so in practice the package file keeps its mode and the target's is discarded.
//   - The destination's **atime and mtime are the source's**, copied afterwards.
//   - The source is unlinked only after both succeed.
//
// A plain rename preserves all of that for free, which is why it is still tried
// first: the fallback runs only when the kernel says the two paths are on
// different filesystems.
func moveFile(src, dst string) error {
	if err := os.Rename(src, dst); err == nil {
		return nil
	} else if !errors.Is(err, syscall.EXDEV) {
		// Perl's _move falls back on *any* rename failure and reports the errno
		// the fallback left behind. Every other rename error (ENOENT, EACCES,
		// ENOSPC) would fail the copy the same way and report the same errno, so
		// returning it here is the same answer by a shorter route.
		return err
	}

	info, err := os.Stat(src)
	if err != nil {
		return err
	}
	if err := copyContents(src, dst); err != nil {
		return err
	}
	// utime($atime, $mtime, $to). Go cannot read atime portably, and stow's own
	// use of it is the mtime; passing the mtime for both matches what a reader of
	// the tree can observe and what `touch -r` would do.
	if err := os.Chtimes(dst, info.ModTime(), info.ModTime()); err != nil {
		return err
	}
	return os.Remove(src)
}

// copyContents is File::Copy::copy for a plain file: the destination is opened
// with O_CREAT|O_TRUNC and mode 0666, so the umask decides a new file's mode and
// an existing file keeps its own.
func copyContents(src, dst string) (err error) {
	in, err := os.Open(src)
	if err != nil {
		return err
	}
	defer func() { _ = in.Close() }()

	out, err := os.OpenFile(dst, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, 0o666)
	if err != nil {
		return err
	}
	defer func() {
		if cerr := out.Close(); err == nil {
			err = cerr
		}
	}()

	_, err = io.Copy(out, in)
	return err
}
