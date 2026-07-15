package stowrc

import (
	"strconv"

	"github.com/rocne/gostow/internal/getopt"
	"github.com/rocne/gostow/stow"
)

// spec mirrors bin/stow's GetOptionsFromArray call verbatim, including the
// canonical names Getopt::Long writes into %options. It is the only place
// gostow's options are declared; [OptionNames] is its public projection.
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
		{Names: []string{"ignore"}, Arg: getopt.StringArg, Validate: patternValidator("ignore", stow.IgnoreAnchor)},
		{Names: []string{"override"}, Arg: getopt.StringArg, Validate: patternValidator("override", stow.PrefixAnchor)},
		{Names: []string{"defer"}, Arg: getopt.StringArg, Validate: patternValidator("defer", stow.PrefixAnchor)},
		{Names: []string{"D", "delete"}},
		{Names: []string{"S", "stow"}},
		{Names: []string{"R", "restow"}},

		// gostow's own extension. Two rules keep it from denting parity: it is
		// prefixed "gostow-", and it is NoAbbrev — so adding it cannot make "--g"
		// resolve to anything real stow would have rejected. It is listed in
		// --help, because a flag nobody can discover is a flag nobody uses, and
		// the DIVERGENCES section of the man page is the full account. The parity
		// suite forbids any fixture from using it.
		{Names: []string{"gostow-fix"}, NoAbbrev: true},
	}
}

// OptionNames lists the option table: one entry per option, canonical name
// first, aliases after it. A single-character name is also a bundlable short
// option. The table itself stays private — its entry type lives in an internal
// package — so this projection is what keeps external references (docs,
// completions, a consumer's knob mapping) in step with what [Parse] accepts.
func OptionNames() [][]string {
	var out [][]string
	for _, opt := range spec() {
		out = append(out, append([]string(nil), opt.Names...))
	}
	return out
}

// patternValidator is the Getopt::Long callback bin/stow gives each of its three
// regex options: it compiles the anchored pattern while parsing, so an
// uncompilable one is a parse failure — diagnostic on stderr, usage block on
// stdout, exit 1 — and not, as it once was in gostow, a fatal error raised much
// later by the engine, after .stowrc, after --dir validation, and after the "No
// packages to stow or unstow" check had already swallowed it.
//
// The diagnostic's *text* cannot match. Perl prints its own regex engine's
// complaint ("Unmatched ( in regex; marked by <-- HERE ..."), naming a line in
// bin/stow. gostow prints Go's. The timing, the streams and the exit code are the
// reproducible behaviour, and those are matched exactly. See SPEC §10, PL-20.
func patternValidator(flag, anchor string) func(string) error {
	return func(value string) error {
		_, err := stow.CompilePattern(flag, anchor, value)
		return err
	}
}

// Parse consumes one token stream — a command line, or the concatenation of rc
// files, which stow parses with the same parser (SPEC §5) — under stow's
// Getopt::Long configuration. It never fails: diagnostics accumulate in
// [Result.Errors] and parsing continues, exactly as GetOptions does. No
// expansion is applied; a caller parsing rc tokens directly applies
// [ExpandFilepath] to Dir and Target afterwards, as [ParseReader] does.
func Parse(tokens []string) Result {
	res := getopt.Parse(spec(), tokens)
	p := Result{Errors: res.Errors, Leftover: res.Leftover}

	action := stow.ActionStow
	addPkg := func(name string) {
		if n := len(p.Requests); n > 0 && p.Requests[n-1].Action == action {
			p.Requests[n-1].Packages = append(p.Requests[n-1].Packages, name)
			return
		}
		p.Requests = append(p.Requests, stow.Request{Action: action, Packages: []string{name}})
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
				p.Verbose = &n
				continue
			}
			if p.Verbose == nil {
				zero := 0
				p.Verbose = &zero
			}
			*p.Verbose++
		case "help":
			p.Help = true
		case "simulate":
			p.Simulate = true
		case "version":
			p.Version = true
		case "compat":
			p.Compat = true
		case "adopt":
			p.Adopt = true
		case "no-folding":
			p.NoFolding = true
		case "dotfiles":
			p.Dotfiles = true
		case "gostow-fix":
			p.FixQuirks = true
		case "dir":
			v := e.Value
			p.Dir = &v
		case "target":
			v := e.Value
			p.Target = &v
		case "ignore":
			p.Ignore = append(p.Ignore, e.Value)
		case "override":
			p.Override = append(p.Override, e.Value)
		case "defer":
			p.Defer = append(p.Defer, e.Value)
		}
	}

	// stow hands the arguments after "--" back in an array it never reads, so
	// `stow -- pkg` silently drops pkg and dies with "No packages to stow or
	// unstow" (ledger PL-03). Fails loudly, but it fails.
	if p.FixQuirks {
		for _, name := range p.Leftover {
			addPkg(name)
		}
		p.Leftover = nil
	}
	return p
}
