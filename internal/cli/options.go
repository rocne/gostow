package cli

import (
	"strconv"
	"strings"

	"github.com/rocne/gostow/internal/getopt"
	"github.com/rocne/gostow/stow"
)

// spec mirrors bin/stow's GetOptionsFromArray call verbatim, including the
// canonical names Getopt::Long writes into %options.
func spec() []getopt.Option {
	return []getopt.Option{
		{Names: []string{"verbose", "v"}, Arg: getopt.OptionalIntArg},
		{Names: []string{"help", "h"}},
		{Names: []string{"simulate", "n", "no"}},
		{Names: []string{"version", "V"}},
		{Names: []string{"compat", "p"}},
		{Names: []string{"dir", "d"}, Arg: getopt.StringArg},
		{Names: []string{"target", "t"}, Arg: getopt.StringArg},
		{Names: []string{"adopt"}},
		{Names: []string{"no-folding"}},
		{Names: []string{"dotfiles"}},
		{Names: []string{"ignore"}, Arg: getopt.StringArg},
		{Names: []string{"override"}, Arg: getopt.StringArg},
		{Names: []string{"defer"}, Arg: getopt.StringArg},
		{Names: []string{"D", "delete"}},
		{Names: []string{"S", "stow"}},
		{Names: []string{"R", "restow"}},

		// gostow's own extensions. Two rules keep them from denting parity: they
		// are prefixed "gostow-", and they are NoAbbrev — so adding them cannot
		// make "--g" resolve to anything real stow would have rejected. They are
		// listed in --help, because a flag nobody can discover is a flag nobody
		// uses; --gostow-help prints the long form, and docs/DIVERGENCES.md is
		// the full account. The parity suite forbids any fixture from using them.
		{Names: []string{"gostow-fix"}, NoAbbrev: true},
		{Names: []string{"gostow-help"}, NoAbbrev: true},
	}
}

// parsed is one option source — the command line, or the concatenated .stowrc
// files. Scalars are pointers so that "absent" is distinguishable from "set to
// the zero value", which is what the rc/CLI merge turns on.
type parsed struct {
	verbose   *int
	help      bool
	simulate  bool
	version   bool
	compat    bool
	adopt     bool
	noFolding bool
	dotfiles  bool
	dir       *string
	target    *string
	ignore    []string
	override  []string
	deferred  []string

	// gostow's own extensions; see spec().
	fixQuirks  bool
	gostowHelp bool

	// requests preserves the order of -D/-S/-R and the package names they
	// govern. rc files parse into these too, and then discard them.
	requests []stow.Request
	// leftover is what followed "--". stow discards it; --gostow-fix keeps it.
	leftover []string
	errors   []string
}

func parseArgs(args []string) parsed {
	res := getopt.Parse(spec(), args)
	p := parsed{errors: res.Errors, leftover: res.Leftover}

	action := stow.ActionStow
	addPkg := func(name string) {
		if n := len(p.requests); n > 0 && p.requests[n-1].Action == action {
			p.requests[n-1].Packages = append(p.requests[n-1].Packages, name)
			return
		}
		p.requests = append(p.requests, stow.Request{Action: action, Packages: []string{name}})
	}

	for _, e := range res.Events {
		switch e.Option {
		case "":
			addPkg(e.Arg)
		case "D":
			action = stow.ActionUnstow
		case "S":
			action = stow.ActionStow
		case "R":
			action = stow.ActionRestow
		case "verbose":
			if e.HasValue {
				n, _ := strconv.Atoi(e.Value) // the parser guarantees an integer
				p.verbose = &n
				continue
			}
			if p.verbose == nil {
				zero := 0
				p.verbose = &zero
			}
			*p.verbose++
		case "help":
			p.help = true
		case "simulate":
			p.simulate = true
		case "version":
			p.version = true
		case "compat":
			p.compat = true
		case "adopt":
			p.adopt = true
		case "no-folding":
			p.noFolding = true
		case "dotfiles":
			p.dotfiles = true
		case "gostow-fix":
			p.fixQuirks = true
		case "gostow-help":
			p.gostowHelp = true
		case "dir":
			v := e.Value
			p.dir = &v
		case "target":
			v := e.Value
			p.target = &v
		case "ignore":
			p.ignore = append(p.ignore, e.Value)
		case "override":
			p.override = append(p.override, e.Value)
		case "defer":
			p.deferred = append(p.deferred, e.Value)
		}
	}

	// stow hands the arguments after "--" back in an array it never reads, so
	// `stow -- pkg` silently drops pkg and dies with "No packages to stow or
	// unstow" (ledger PL-03). Fails loudly, but it fails.
	if p.fixQuirks {
		for _, name := range p.leftover {
			addPkg(name)
		}
		p.leftover = nil
	}
	return p
}

// merge combines the rc options with the command line. For list-valued options
// the rc entries come first and the CLI's are appended; for scalars the CLI
// overwrites. Booleans are a union, since an unset boolean is indistinguishable
// from a false one in stow's %options too.
func merge(rc, cli parsed) parsed {
	out := cli
	out.ignore = append(append([]string{}, rc.ignore...), cli.ignore...)
	out.override = append(append([]string{}, rc.override...), cli.override...)
	out.deferred = append(append([]string{}, rc.deferred...), cli.deferred...)

	if out.dir == nil {
		out.dir = rc.dir
	}
	if out.target == nil {
		out.target = rc.target
	}
	if out.verbose == nil {
		out.verbose = rc.verbose
	}
	out.simulate = cli.simulate || rc.simulate
	out.compat = cli.compat || rc.compat
	out.adopt = cli.adopt || rc.adopt
	out.noFolding = cli.noFolding || rc.noFolding
	out.dotfiles = cli.dotfiles || rc.dotfiles
	out.fixQuirks = cli.fixQuirks || rc.fixQuirks
	return out
}

func (p parsed) verbosity() int {
	if p.verbose == nil {
		return 0
	}
	return *p.verbose
}

// packages flattens the requests in order, which is what check_packages walks.
func (p parsed) packages() []string {
	var out []string
	for _, r := range p.requests {
		out = append(out, r.Packages...)
	}
	return out
}

// stripTrailingSlashes mirrors check_packages' in-place `s{/+$}{}`, which exists
// so that shell tab-completion's "pkg/" works.
func stripTrailingSlashes(p *parsed) {
	for i := range p.requests {
		for j, name := range p.requests[i].Packages {
			p.requests[i].Packages[j] = strings.TrimRight(name, "/")
		}
	}
}
