package cli

import (
	"strings"

	"github.com/rocne/gostow/stowrc"
)

// The option table and the token parser live in the public stowrc package —
// stow parses the command line and the rc concatenation with the same parser,
// so exposing rc parsing (issue #39) moved the parser wholesale. What stays
// here is what only the CLI does with a parse: merging the two sources and
// vetting package names.

// merge combines the rc options with the command line. For list-valued options
// the rc entries come first and the CLI's are appended; for scalars the CLI
// overwrites. Booleans are a union, since an unset boolean is indistinguishable
// from a false one in stow's %options too.
func merge(rc, cli stowrc.Result) stowrc.Result {
	out := cli
	out.Ignore = append(append([]string{}, rc.Ignore...), cli.Ignore...)
	out.Override = append(append([]string{}, rc.Override...), cli.Override...)
	out.Defer = append(append([]string{}, rc.Defer...), cli.Defer...)

	if out.Dir == nil {
		out.Dir = rc.Dir
	}
	if out.Target == nil {
		out.Target = rc.Target
	}
	if out.Verbose == nil {
		out.Verbose = rc.Verbose
	}
	out.Simulate = cli.Simulate || rc.Simulate
	out.Compat = cli.Compat || rc.Compat
	out.Adopt = cli.Adopt || rc.Adopt
	out.NoFolding = cli.NoFolding || rc.NoFolding
	out.Dotfiles = cli.Dotfiles || rc.Dotfiles
	out.FixQuirks = cli.FixQuirks || rc.FixQuirks
	return out
}

// packages flattens the requests in order, which is what check_packages walks.
func packages(p stowrc.Result) []string {
	var out []string
	for _, r := range p.Requests {
		out = append(out, r.Packages...)
	}
	return out
}

// stripTrailingSlashes mirrors check_packages' in-place `s{/+$}{}`, which exists
// so that shell tab-completion's "pkg/" works.
func stripTrailingSlashes(p *stowrc.Result) {
	for i := range p.Requests {
		for j, name := range p.Requests[i].Packages {
			p.Requests[i].Packages[j] = strings.TrimRight(name, "/")
		}
	}
}
