//go:build oracle

package conformance

import "testing"

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
		{Name: "no packages", Stow: Tree{"pkg/f": F("x")}, Args: base},
		{Name: "unknown option", Stow: Tree{"pkg/f": F("x")}, Args: args("--bogus", "pkg")},
		{Name: "ambiguous abbreviation", Stow: Tree{"pkg/f": F("x")}, Args: args("--ver", "pkg")},
		{Name: "bad --dir", Args: []string{"-d", "nosuchdir", "-t", "target", "pkg"}},
		{Name: "bad --target", Args: []string{"-d", "stow", "-t", "nosuchdir", "pkg"}},
		{
			Name: "-- discards the packages after it",
			Stow: Tree{"pkg/f": F("x")},
			Args: args("--", "pkg"),
		},

		// --- help and version (identity line normalised away) ---------------
		{Name: "--help", Args: []string{"--help"}},
		{Name: "-h", Args: []string{"-h"}},
		{Name: "--version", Args: []string{"--version"}},
		{Name: "help beats version", Args: []string{"-V", "-h"}},

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
		{
			Name: "--compat unstow walks the target tree",
			Stow: Tree{"pkg/f": F("x")},
			Args: args("-v", "-p", "-D", "pkg"),
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
