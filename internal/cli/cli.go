// Package cli is gostow's command-line front end: option resolution, .stowrc
// merging, the usage block, conflict reporting, and exit codes.
//
// It is separate from package stow because the engine is a library dstow will
// call, and none of this belongs to it. stow draws the same line: bin/stow does
// option handling and conflict printing, Stow.pm does the symlink farm.
package cli

import (
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/rocne/gostow/internal/stowpath"
	"github.com/rocne/gostow/internal/ui"
	"github.com/rocne/gostow/stow"
)

// Exit codes. Fatal errors are pinned to 2 rather than following stow's
// errno-derived status, which is undefined — ledger PL-07.
const (
	exitOK    = 0
	exitUsage = 1
	exitFatal = 2
)

type app struct {
	prog    string
	version string
	stdout  io.Writer
	stderr  io.Writer
}

// Run is the whole CLI. args is os.Args. It never calls os.Exit, so it is
// testable in-process; the caller propagates the status.
func Run(args []string, version string, stdout, stderr io.Writer) int {
	// stow's $ProgramName is basename($0), not a constant: installed as `stow`,
	// gostow's usage errors and synopsis are byte-identical. Ledger PL-17.
	prog := "gostow"
	if len(args) > 0 {
		prog = filepath.Base(args[0])
	}

	// Colour, gostow's one addition to stow's output, is applied here and only
	// here: both streams are wrapped once, so the engine's operation log (which
	// is handed a.stderr as its Log) is painted without package stow knowing
	// that terminals exist. Off a TTY these are byte pass-throughs. SPEC §8.4.
	out := ui.NewWriter(stdout, ui.Enabled(stdout))
	errOut := ui.NewWriter(stderr, ui.Enabled(stderr))
	defer func() { _ = out.Flush() }()
	defer func() { _ = errOut.Flush() }()

	a := &app{prog: prog, version: version, stdout: out, stderr: errOut}

	code, err := a.run(args[1:])
	if err != nil {
		// stow has two fatal paths with different output. Stow::Util::error()
		// prefixes "<prog>: ERROR: "; a bare die() does not. Both are pinned to
		// exit 2 here, because stow's die status is errno-derived and therefore
		// undefined (ledger PL-07) -- but the message bytes are reproducible and
		// so are replicated exactly.
		var de *dieError
		if errors.As(err, &de) {
			fmt.Fprintln(a.stderr, de.msg)
		} else {
			fmt.Fprintf(a.stderr, "%s: ERROR: %v\n", a.prog, err)
		}
		return exitFatal
	}
	return code
}

// dieError is a Perl die(): its message reaches stderr unadorned.
type dieError struct{ msg string }

func (e *dieError) Error() string { return e.msg }

// usage prints the help block on stdout and, when msg is non-empty, a diagnostic
// on stderr first. stow calls usage(”) on a parse failure: the empty message
// prints nothing but still exits 1, so an absent message and an empty one are
// different things.
func (a *app) usage(msg string, showMsg bool) int {
	if showMsg {
		if msg != "" {
			fmt.Fprintf(a.stderr, "%s: %s\n\n", a.prog, msg)
		}
		fmt.Fprint(a.stdout, usageText(a.prog, a.version))
		return exitUsage
	}
	fmt.Fprint(a.stdout, usageText(a.prog, a.version))
	return exitOK
}

func (a *app) run(argv []string) (int, error) {
	cli := parseArgs(argv)
	if code, done := a.finishParse(cli); done {
		return code, nil
	}

	rcTokens, err := readStowrcTokens(cli.fixQuirks)
	if err != nil {
		return 0, err
	}
	rc := parseArgs(rcTokens)
	if code, done := a.finishParse(rc); done {
		return code, nil
	}
	// rc package names are parsed and then discarded, which is what makes
	// .stowrc comments appear to work (ledger PL-02).
	rc.requests = nil

	if rc.dir != nil {
		expanded, err := expandFilepath(*rc.dir, "--dir option")
		if err != nil {
			return 0, err
		}
		rc.dir = &expanded
	}
	if rc.target != nil {
		expanded, err := expandFilepath(*rc.target, "--target option")
		if err != nil {
			return 0, err
		}
		rc.target = &expanded
	}

	opts := merge(rc, cli)

	dir, target, code, err := a.sanitizePaths(&opts)
	if err != nil || code != exitOK {
		return code, err
	}
	if code, err := a.checkPackages(&opts); err != nil || code != exitOK {
		return code, err
	}

	return a.apply(opts, dir, target)
}

// finishParse reports parse diagnostics and handles --help/--version, which stow
// honours only after a successful parse. help wins over version: usage() is
// checked first.
func (a *app) finishParse(p parsed) (int, bool) {
	if len(p.errors) > 0 {
		for _, e := range p.errors {
			fmt.Fprintln(a.stderr, e)
		}
		return a.usage("", true), true
	}
	if p.help {
		return a.usage("", false), true
	}
	if p.version {
		fmt.Fprintln(a.stdout, IdentityLine(a.version))
		return exitOK, true
	}
	return 0, false
}

// sanitizePaths resolves --dir from $STOW_DIR or the cwd, and --target from the
// parent of --dir, validating both. Because this runs *after* the rc/CLI merge,
// a --dir in .stowrc beats $STOW_DIR.
func (a *app) sanitizePaths(p *parsed) (dir, target string, code int, err error) {
	if p.dir != nil {
		dir = *p.dir
	} else if sd := os.Getenv("STOW_DIR"); sd != "" {
		dir = sd
	} else {
		cwd, err := os.Getwd()
		if err != nil {
			return "", "", 0, err
		}
		dir = cwd
	}
	if !isDir(dir) {
		return "", "", a.usage(fmt.Sprintf("--dir value '%s' is not a valid directory", dir), true), nil
	}

	if p.target != nil {
		target = *p.target
		if !isDir(target) {
			return "", "", a.usage(fmt.Sprintf("--target value '%s' is not a valid directory", target), true), nil
		}
	} else {
		// bin/stow: $target = parent($stow_dir) || '.'. Parent("/tmp") is "",
		// not "/", so a single-segment stow dir targets the cwd.
		target = stowpath.Parent(dir)
		if target == "" {
			target = "."
		}
	}
	return dir, target, exitOK, nil
}

func isDir(p string) bool {
	fi, err := os.Stat(p)
	return err == nil && fi.IsDir()
}

func (a *app) checkPackages(p *parsed) (int, error) {
	if len(p.packages()) == 0 {
		return a.usage("No packages to stow or unstow", true), nil
	}
	stripTrailingSlashes(p)
	for _, name := range p.packages() {
		if strings.Contains(name, "/") {
			return 0, errors.New("Slashes are not permitted in package names") //nolint:staticcheck // byte-exact stow wording
		}
	}
	return exitOK, nil
}

func (a *app) apply(p parsed, dir, target string) (int, error) {
	opts := stow.Options{
		Dir:       dir,
		Target:    target,
		Fold:      !p.noFolding,
		Dotfiles:  p.dotfiles,
		Adopt:     p.adopt,
		Compat:    p.compat,
		Simulate:  p.simulate,
		FixQuirks: p.fixQuirks,
		Verbosity: p.verbosity(),
		Ignore:    p.ignore,
		Defer:     p.deferred,
		Override:  p.override,
		Log:       a.stderr,
	}

	_, err := stow.Apply(opts, p.requests...)

	var ce *stow.ConflictError
	if errors.As(err, &ce) {
		a.reportConflicts(ce.Conflicts)
		return exitUsage, nil
	}
	if err != nil {
		return 0, err
	}

	if p.simulate {
		fmt.Fprintln(a.stderr, "WARNING: in simulation mode so not modifying filesystem.")
	}
	return exitOK, nil
}

// reportConflicts prints unstow conflicts before stow conflicts, packages sorted
// within an action, and messages sorted within a package.
func (a *app) reportConflicts(conflicts []stow.Conflict) {
	byAction := map[stow.Action]map[string][]string{}
	for _, c := range conflicts {
		if byAction[c.Action] == nil {
			byAction[c.Action] = map[string][]string{}
		}
		byAction[c.Action][c.Package] = append(byAction[c.Action][c.Package], c.Message)
	}

	for _, action := range []stow.Action{stow.ActionUnstow, stow.ActionStow} {
		pkgs := byAction[action]
		if len(pkgs) == 0 {
			continue
		}
		names := make([]string, 0, len(pkgs))
		for name := range pkgs {
			names = append(names, name)
		}
		sort.Strings(names)
		for _, name := range names {
			fmt.Fprintf(a.stderr, "WARNING! %s %s would cause conflicts:\n", stow.Gerund(action), name)
			msgs := pkgs[name]
			sort.Strings(msgs)
			for _, m := range msgs {
				fmt.Fprintf(a.stderr, "  * %s\n", m)
			}
		}
	}
	fmt.Fprintln(a.stderr, "All operations aborted.")
}
