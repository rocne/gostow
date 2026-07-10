package stow

import (
	"errors"
	"syscall"
	"unicode"
	"unicode/utf8"
)

// errnoText renders err the way Perl renders $!: the C library's strerror string
// for the errno, whose first letter is capitalised.
//
// stow interpolates $! straight into its fatal messages — "cannot read directory:
// ../stow/pkg/sub (Permission denied)". Go's error string for the same failure is
// "open /abs/path/stow/pkg/sub: permission denied": it names the syscall, repeats
// the path, and repeats it *absolutely*, in a message whose other path is
// target-relative. Formatting the error with %v therefore broke parity twice over
// and leaked the sandbox root into gostow's output.
//
// Go's per-errno strings come from the same table as strerror and differ only in
// case, so capitalising the first rune is the whole translation. That also keeps
// this correct off glibc: on darwin Go reports darwin's wording, which is what
// real stow would print there too.
func errnoText(err error) string {
	var errno syscall.Errno
	if !errors.As(err, &errno) {
		return err.Error()
	}
	s := errno.Error()
	r, size := utf8.DecodeRuneInString(s)
	if r == utf8.RuneError {
		return s
	}
	return string(unicode.ToUpper(r)) + s[size:]
}
