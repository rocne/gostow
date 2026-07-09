//go:build oracle

package conformance

import (
	"testing"

	// Imported for the dependency edge, not for anything it exports.
	//
	// These tests drive gostow by building a binary at run time and execing it,
	// so nothing here would otherwise reference the code under test. `go test`
	// caches a package's result against its import graph: with no edge to
	// internal/cli, editing cli.go leaves the graph unchanged and Go replays the
	// previous PASS without running anything. A differential suite that reports
	// success without executing is the purest form of the vacuous pass this
	// project keeps finding — caught by mutating cli.go's usage path and watching
	// six fixtures stay green.
	//
	// internal/cli transitively pulls in internal/getopt, internal/ui and stow:
	// everything cmd/gostow is made of but its three-line main().
	//
	// It belongs in a _test.go file, not in gostow.go. Package stow's own oracle
	// tests import this package, and internal/cli imports stow — a non-test import
	// here would be an import cycle. A package's test files are not compiled into
	// it when another package imports it, so this edge exists only where it is
	// needed.
	_ "github.com/rocne/gostow/internal/cli"
)

// The full-binary differential suite (seam S2): the same argv, the same fixture,
// real stow 2.4.1 on one copy and gostow on the other, comparing stdout bytes,
// stderr bytes, exit code and the resulting tree.
//
// gostow's binary is built as `stow`, because stow's program name is basename($0)
// (ledger PL-17); that makes usage errors and the synopsis directly comparable.
func TestCLIAgainstOracle(t *testing.T) {
	base := []string{"-d", "stow", "-t", "target"}
	args := func(rest ...string) []string { return append(append([]string{}, base...), rest...) }

	cases := []Case{
		// --- default directory resolution -----------------------------------
		//
		// Every other fixture passes "-d stow -t target", so until these existed
		// the commonest invocation in the world -- `cd ~/dotfiles && stow vim` --
		// was never compared against the oracle. The gap hid a real bug: the CLI
		// carried its own copy of Stow::Util::parent, and on a single-segment
		// absolute stow dir it returned "/" where Perl returns "", so
		// `stow -d /tmp pkg` aimed the symlink farm at the filesystem root.
		//
		// `-d` defaults to the cwd; `-t` defaults to parent($stow_dir) || '.'.
		{
			Name: "no --dir and no --target: stow dir is the cwd, target its parent",
			Stow: Tree{"pkg/f": F("x")},
			Cwd:  "stow",
			Args: []string{"-v", "pkg"},
		},
		{
			Name: "no --target: derived from a relative --dir",
			Stow: Tree{"pkg/f": F("x")},
			Args: []string{"-v", "-d", "stow", "pkg"},
		},
		{
			Name: "no --target: derived from an absolute --dir",
			Stow: Tree{"pkg/f": F("x")},
			Args: []string{"-v", "-d", SandboxToken + "/stow", "pkg"},
		},
		{
			Name: "no --dir and no --target, unstowing again",
			Stow: Tree{"pkg/f": F("x")},
			Cwd:  "stow",
			Pre:  []string{"-d", ".", "pkg"},
			Args: []string{"-v", "-D", "pkg"},
		},

		// --- the operation log at each verbosity ---------------------------
		{
			Name: "stow one file, silent at verbosity 0",
			Stow: Tree{"pkg/f": F("x")},
			Args: args("pkg"),
		},
		{
			Name: "stow one file at -v",
			Stow: Tree{"pkg/f": F("x")},
			Args: args("-v", "pkg"),
		},
		{
			Name: "stow one file at -vv",
			Stow: Tree{"pkg/f": F("x")},
			Args: args("-vv", "pkg"),
		},
		{
			Name: "simulate prints its warning and writes nothing",
			Stow: Tree{"pkg/f": F("x")},
			Args: args("-n", "pkg"),
		},
		{
			Name: "simulate with -nv shows the plan",
			Stow: Tree{"pkg/sub/a": F("a"), "pkg/sub/b": F("b")},
			Args: args("-nv", "pkg"),
		},

		// --- conflicts -----------------------------------------------------
		{
			Name:   "conflict report and exit 1",
			Stow:   Tree{"pkg/f": F("new")},
			Target: Tree{"f": F("old")},
			Args:   args("pkg"),
		},
		{
			Name:   "two conflicts are sorted within a package",
			Stow:   Tree{"pkg/a": F("a"), "pkg/b": F("b")},
			Target: Tree{"a": F("a"), "b": F("b")},
			Args:   args("pkg"),
		},

		// --- fatal errors --------------------------------------------------
		{
			Name: "missing package is fatal",
			Stow: Tree{"pkg/f": F("x")},
			Args: args("nope"),
		},
		{
			Name: "a slash in a package name is fatal",
			Stow: Tree{"pkg/f": F("x")},
			Args: args("pkg/sub"),
		},
		{
			Name: "a trailing slash is stripped, not fatal",
			Stow: Tree{"pkg/f": F("x")},
			Args: args("-v", "pkg/"),
		},

		// --- usage errors: message on stderr, usage on stdout, exit 1 -------
		//
		// The diagnostic and the exit code are the contract and are compared byte
		// for byte. The help block dumped on stdout is prose, and gostow's prose is
		// its own; UsageOnStdout requires each binary to print exactly its own
		// --help there. See SPEC §4.5.
		{Name: "no packages", Stow: Tree{"pkg/f": F("x")}, Args: base, UsageOnStdout: true},
		{Name: "unknown option", Stow: Tree{"pkg/f": F("x")}, Args: args("--bogus", "pkg"), UsageOnStdout: true},
		{Name: "ambiguous abbreviation", Stow: Tree{"pkg/f": F("x")}, Args: args("--ver", "pkg"), UsageOnStdout: true},
		{Name: "bad --dir", Args: []string{"-d", "nosuchdir", "-t", "target", "pkg"}, UsageOnStdout: true},
		{Name: "bad --target", Args: []string{"-d", "stow", "-t", "nosuchdir", "pkg"}, UsageOnStdout: true},
		{
			Name:          "-- discards the packages after it",
			Stow:          Tree{"pkg/f": F("x")},
			Args:          args("--", "pkg"),
			UsageOnStdout: true,
		},

		// --- help and version -----------------------------------------------
		//
		// --help itself is not a byte fixture: the blocks differ by design. What
		// is checked instead lives in TestHelpDocumentsEveryOptionStowDocuments
		// below (the interface) and in package cli (the prose). --version stays a
		// byte fixture: after the identity line is normalised it must be empty,
		// which pins the stream and the exit code.
		{Name: "--version", Args: []string{"--version"}},
		{Name: "help beats version", Args: []string{"-V", "-h"}, UsageOnStdout: true},

		// --- option semantics reaching the engine ---------------------------
		{
			Name: "--no-folding",
			Stow: Tree{"pkg/sub/a": F("a")},
			Args: args("-v", "--no-folding", "pkg"),
		},
		{
			Name: "--dotfiles",
			Stow: Tree{"pkg/dot-bashrc": F("rc")},
			Args: args("-v", "--dotfiles", "pkg"),
		},
		{
			Name: "--ignore is a suffix match",
			Stow: Tree{"pkg/keep": F("k"), "pkg/drop.log": F("d")},
			Args: args("-v", "--ignore=log", "pkg"),
		},
		{
			Name: "-D unstows, -S stows, in one invocation",
			Stow: Tree{"one/a": F("a"), "two/b": F("b")},
			Args: args("-v", "-S", "two"),
		},
		{
			Name: "permuted options and packages",
			Stow: Tree{"pkg/f": F("x")},
			Args: append(args("pkg"), "-v"),
		},

		// --- .stowrc --------------------------------------------------------
		{
			Name: "rc supplies --dir and --target",
			Stow: Tree{"pkg/f": F("x")},
			Rc:   "--dir=stow\n--target=target\n",
			Args: []string{"-v", "pkg"},
		},
		{
			Name: "cli --target overrides the rc value",
			Stow: Tree{"pkg/f": F("x")},
			Rc:   "--dir=stow\n--target=nosuchdir\n",
			Args: []string{"-v", "-t", "target", "pkg"},
		},
		{
			Name: "rc ignore patterns come before cli ones and both apply",
			Stow: Tree{"pkg/keep": F("k"), "pkg/a.log": F("a"), "pkg/b.tmp": F("b")},
			Rc:   "--ignore=log\n",
			Args: append(args("-v", "--ignore=tmp"), "pkg"),
		},
		{
			Name: "rc package names are parsed and discarded",
			Stow: Tree{"pkg/f": F("x")},
			Rc:   "--dir=stow --target=target notapackage\n",
			Args: []string{"-v", "pkg"},
		},
		{
			Name: "an rc comment works only by accident, ledger PL-02",
			Stow: Tree{"pkg/f": F("x")},
			Rc:   "--dir=stow --target=target # this is not a comment syntax\n",
			Args: []string{"-v", "pkg"},
		},
		{
			Name: "rc --target expands an environment variable",
			Stow: Tree{"pkg/f": F("x")},
			Rc:   "--dir=stow\n--target=$GOSTOW_TGT\n",
			Env:  []string{"GOSTOW_TGT=" + SandboxToken + "/target"},
			Args: []string{"-v", "pkg"},
		},
		{
			Name: "rc --target referencing an undefined variable is fatal",
			Stow: Tree{"pkg/f": F("x")},
			Rc:   "--dir=stow\n--target=$GOSTOW_UNDEFINED\n",
			Args: []string{"pkg"},
			// stow die()s here, so its status is whatever errno was left behind.
			FatalExitDiverges: true,
		},
		{
			Name: "home rc is read before the cwd rc, so cwd wins, ledger PL-01",
			Stow: Tree{"pkg/f": F("x")},
			// If home won, --target would be the invalid one and stow would fail.
			HomeRc: "--target=nosuchdir\n",
			Rc:     "--dir=stow\n--target=target\n",
			Args:   []string{"-v", "pkg"},
		},

		// --- protected directories -------------------------------------------
		{
			Name:   "a .stow-marked target directory is skipped",
			Stow:   Tree{"pkg/sub/a": F("a")},
			Target: Tree{"sub/.stow": F("")},
			Args:   args("-v", "pkg"),
		},
		{
			Name:   "a .nonstow-marked target directory is skipped",
			Stow:   Tree{"pkg/sub/a": F("a")},
			Target: Tree{"sub/.nonstow": F("")},
			Args:   args("-v", "pkg"),
		},

		// --- --compat ---------------------------------------------------------
		//
		// A discriminating fixture (SPEC §12 owed one). The package's file was
		// renamed f -> g, leaving target/f dangling into the package. Both modes
		// end with the same tree, so only the level-2 log tells them apart:
		// walking the *package* tree never visits f, and it is instead swept up
		// afterwards by cleanup_invalid_links; walking the *target* tree finds f
		// directly and calls it an invalid link into a stow directory. compat
		// never runs cleanup_invalid_links at all.
		{
			Name:   "unstow walks the package tree",
			Stow:   Tree{"pkg/g": F("g")},
			Target: Tree{"f": L("../stow/pkg/f")},
			Args:   args("-vv", "-D", "pkg"),
		},
		{
			Name:   "--compat unstow walks the target tree instead",
			Stow:   Tree{"pkg/g": F("g")},
			Target: Tree{"f": L("../stow/pkg/f")},
			Args:   args("-vv", "-p", "-D", "pkg"),
		},
		{
			Name: "--compat unstow of a plain package",
			Stow: Tree{"pkg/f": F("x")},
			Args: args("-v", "-p", "-D", "pkg"),
		},

		// --- ledger PL-04: the protection asymmetry ---------------------------
		//
		// stow_contents passes the *package* subdir to should_skip_target while
		// unstow_contents passes the *target* subdir. Under --dotfiles those are
		// different names, so stowing into a .stow-marked directory silently
		// bypasses the protection that unstowing honours. Replicated for v1.
		{
			Name:   "PL-04: --dotfiles bypasses .stow protection when stowing",
			Stow:   Tree{"pkg/dot-foo/bar": F("x")},
			Target: Tree{".foo/.stow": F("")},
			Args:   args("-v", "--dotfiles", "pkg"),
		},
		{
			Name:   "PL-04: unstowing the same tree honours the protection",
			Stow:   Tree{"pkg/dot-foo/bar": F("x")},
			Target: Tree{".foo/.stow": F("")},
			Pre:    []string{"-d", "stow", "-t", "target", "--dotfiles", "pkg"},
			Args:   args("-v", "--dotfiles", "-D", "pkg"),
		},

		// --- STOW_DIR --------------------------------------------------------
		{
			Name: "STOW_DIR supplies the stow directory",
			Stow: Tree{"pkg/f": F("x")},
			Env:  []string{"STOW_DIR=" + SandboxToken + "/stow"},
			Args: []string{"-v", "-t", "target", "pkg"},
		},
		{
			Name: "rc --dir beats STOW_DIR",
			Stow: Tree{"pkg/f": F("x")},
			Rc:   "--dir=stow\n",
			Env:  []string{"STOW_DIR=" + SandboxToken + "/nosuchdir"},
			Args: []string{"-v", "-t", "target", "pkg"},
		},
	}

	for _, c := range cases {
		t.Run(c.Name, func(t *testing.T) { AssertSameAsOracle(t, c) })
	}
}
