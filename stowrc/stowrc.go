// Package stowrc parses GNU Stow rc files (.stowrc) with stow 2.4.1's exact,
// conformance-tested semantics, exposed for library consumers that need the
// parse result as data rather than as applied configuration.
//
// The pipeline is stow's own, stage for stage:
//
//  1. Each line is split with Perl's Text::ParseWords::shellwords ([Tokens]).
//     '#' is not special, so a ".stowrc comment" only appears to work — see
//     ledger PL-02 in docs/SPEC.md §10; the fixQuirks toggle opts into real
//     comment handling.
//  2. The token stream is parsed with stow's Getopt::Long configuration
//     ([Parse]): bundling, permute, auto-abbrev, and the ":+" optional-integer
//     dialect. Package-name and action tokens are surfaced in
//     [Result.Requests]; stow itself discards them for rc sources, which is
//     the caller's choice to replicate.
//  3. --dir and --target undergo stow's environment-variable and tilde
//     expansion post-parse ([ExpandFilepath]); a reference to an undefined
//     variable is fatal, byte-for-byte as stow dies.
//
// [ParseFile] and [ParseReader] run the whole pipeline over one rc source.
// Discovery stays caller-side: gostow's CLI finds ~/.stowrc and ./.stowrc
// itself, concatenates their token streams, and parses the concatenation once
// — as stow does, so an option at the end of one file may legally take its
// value from the first token of the next. A consumer slotting *specific* files
// calls [ParseFile] per file instead.
//
// Failure shapes are stow's. Perl's readline poisons its handle, so a file
// that opens but cannot be read (a directory, for instance: open(2) succeeds
// and the first read returns EISDIR) makes stow die "Could not close open
// file: ..." — reproduced here as a [DieError], the error type for every
// bare-die path. Getopt diagnostics are not errors: stow collects them and
// keeps parsing, so they land in [Result.Errors].
package stowrc

import (
	"io"
	"os"

	"github.com/rocne/gostow/stow"
)

// Result is one parsed option source — an rc file, or a command line — as
// structured data. Scalars are pointers so that "absent" is distinguishable
// from "set to the zero value", which is what rc/CLI merging and per-knob
// diffing turn on.
type Result struct {
	Verbose   *int
	Help      bool
	Simulate  bool
	Version   bool
	Compat    bool
	Adopt     bool
	NoFolding bool
	Dotfiles  bool
	Dir       *string
	Target    *string
	Ignore    []string
	Override  []string
	Defer     []string

	// FixQuirks reports --gostow-fix, gostow's own extension; see SPEC §8.36.
	FixQuirks bool

	// Requests preserves the order of -D/-S/-R and the package names they
	// govern. stow parses these out of an rc file and then discards them —
	// which is why .stowrc comments appear to work (ledger PL-02) — but they
	// are surfaced here so the caller owns that decision.
	Requests []stow.Request
	// Leftover is what followed "--". stow discards it (ledger PL-03);
	// FixQuirks keeps it as package names.
	Leftover []string
	// Errors holds Getopt::Long's diagnostics, in the order the offending
	// tokens appeared. stow prints them all and then its usage block.
	Errors []string
}

// Verbosity returns the parsed verbose level, 0 when --verbose never appeared.
func (r Result) Verbosity() int {
	if r.Verbose == nil {
		return 0
	}
	return *r.Verbose
}

// DieError is a Perl die(): stow prints its message on stderr unadorned — no
// "prog: ERROR:" prefix — and exits. Every bare-die path in rc handling (an
// unreadable rc file, an undefined environment variable in --dir/--target)
// returns one; message bytes are parity-pinned.
type DieError struct{ Msg string }

func (e *DieError) Error() string { return e.Msg }

// ParseFile parses one rc file: tokenize, parse, expand --dir/--target. An
// error opening the file is returned as-is — the caller named a specific file,
// so the silent skip stow applies during *discovery* does not apply here.
// Parse diagnostics are data, in [Result.Errors]; the error return carries
// only I/O and die-shaped failures.
func ParseFile(path string, fixQuirks bool) (Result, error) {
	f, err := os.Open(path)
	if err != nil {
		return Result{}, err
	}
	defer func() { _ = f.Close() }()
	return ParseReader(f, path, fixQuirks)
}

// ParseReader is [ParseFile] over an already-open source. name stands in for
// the file path in error messages, whose bytes are parity-pinned.
func ParseReader(r io.Reader, name string, fixQuirks bool) (Result, error) {
	tokens, err := Tokens(r, name, fixQuirks)
	if err != nil {
		return Result{}, err
	}
	p := Parse(tokens)
	if p.Dir != nil {
		expanded, err := ExpandFilepath(*p.Dir, "--dir option")
		if err != nil {
			return Result{}, err
		}
		p.Dir = &expanded
	}
	if p.Target != nil {
		expanded, err := ExpandFilepath(*p.Target, "--target option")
		if err != nil {
			return Result{}, err
		}
		p.Target = &expanded
	}
	return p, nil
}
