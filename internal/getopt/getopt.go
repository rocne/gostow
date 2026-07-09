// Package getopt is a Getopt::Long-compatible option parser, scoped to the
// dialect GNU Stow 2.4.1 configures:
//
//	Getopt::Long::config('no_ignore_case', 'bundling', 'permute');
//
// It exists because pflag and cobra cannot express that dialect. Four of its
// behaviours have no equivalent in either library:
//
//   - bundling: "-nv" is "-n -v", and "-ttgt" is "-t tgt".
//   - permute: options and packages may interleave; "stow pkg -n" parses.
//   - auto_abbrev: any unambiguous prefix of a long name works ("--targ"),
//     but an exact name or alias always beats a prefix ("--no" is --simulate,
//     not an ambiguous prefix of --no-folding).
//   - ":+": a bare "-v" increments, "--verbose=2" sets, and "-v 2" also sets
//     because an optional integer argument swallows a numeric next argument.
//
// Parsing never stops at the first error. Getopt::Long collects diagnostics and
// keeps going, so "-vabc" both increments verbose and reports three unknown
// options; stow prints them all, then its usage block. Errors carry the exact
// upstream wording because they are part of the byte-parity contract (SPEC §8).
//
// Every behaviour encoded here was probed against real Getopt::Long, not read
// from its documentation. See getopt_test.go for the case table.
package getopt

import (
	"fmt"
	"regexp"
	"sort"
	"strings"
)

// ArgKind is what an option does with an argument.
type ArgKind int

const (
	// NoArg is a plain flag: 'help|h'.
	NoArg ArgKind = iota
	// StringArg requires a value: 'dir|d=s'.
	StringArg
	// OptionalIntArg is ':+' — an optional integer that sets, or increments
	// when absent: 'verbose|v:+'.
	OptionalIntArg
)

// Option is one entry of a Getopt::Long spec string. Names[0] is canonical; the
// rest are aliases. A single-character name is additionally a bundlable short
// option, and — as Getopt::Long does — remains matchable in long form, which is
// why "--d" is a valid spelling of "-d".
type Option struct {
	Names []string
	Arg   ArgKind
}

// Event is one parsed token, in the order it appeared. Order is load-bearing:
// stow's -D/-S/-R switch the action applied to the package names that follow,
// so a map of options would lose the interleaving.
type Event struct {
	// Option is the canonical name, or "" when this is a positional argument.
	Option string
	// Arg is the positional's text, when Option is "".
	Arg string
	// Value is the option's argument. For OptionalIntArg, HasValue false means
	// "increment"; the value is otherwise a valid integer literal.
	Value    string
	HasValue bool
}

// Result is a whole parse. Errors being non-empty is what stow treats as
// GetOptions() returning false — it still uses the events it did collect.
type Result struct {
	Events []Event
	// Leftover holds arguments after "--". stow passes a throwaway array to
	// GetOptionsFromArray and never reads it back, which is why `stow -- pkg`
	// silently drops pkg and dies with "No packages to stow or unstow".
	// See ledger PL-03.
	Leftover []string
	Errors   []string
}

// OK reports whether parsing produced no diagnostics.
func (r Result) OK() bool { return len(r.Errors) == 0 }

// intLiteral is Getopt::Long's notion of an integer argument.
var intLiteral = regexp.MustCompile(`^[-+]?[0-9]+$`)

type table struct {
	byName  map[string]*Option
	byShort map[byte]*Option
	names   []string
}

func newTable(opts []Option) *table {
	t := &table{byName: map[string]*Option{}, byShort: map[byte]*Option{}}
	for i := range opts {
		o := &opts[i]
		for _, n := range o.Names {
			t.byName[n] = o
			t.names = append(t.names, n)
			if len(n) == 1 {
				t.byShort[n[0]] = o
			}
		}
	}
	sort.Strings(t.names)
	return t
}

// resolve maps a long-form spelling to a table key. An exact hit wins outright,
// so "--d" is the alias d and not an ambiguous prefix of dir/defer/delete. Only
// then is prefix matching tried, and a unique hit replaces the typed name — this
// is why "--tar" reports itself as "target" in diagnostics but "--d" reports "d".
func (t *table) resolve(typed string) (key string, opt *Option, err string) {
	if o, ok := t.byName[typed]; ok {
		return typed, o, ""
	}
	var hits []string
	for _, n := range t.names {
		if strings.HasPrefix(n, typed) {
			hits = append(hits, n)
		}
	}
	// Prefixes of several aliases of the same option are not ambiguous.
	if len(hits) > 1 {
		distinct := map[*Option]bool{}
		for _, h := range hits {
			distinct[t.byName[h]] = true
		}
		if len(distinct) > 1 {
			return "", nil, fmt.Sprintf("Option %s is ambiguous (%s)", typed, strings.Join(hits, ", "))
		}
	}
	if len(hits) == 0 {
		return "", nil, "Unknown option: " + typed
	}
	return hits[0], t.byName[hits[0]], ""
}

// Parse consumes args the way GetOptionsFromArray does under stow's config.
func Parse(opts []Option, args []string) Result {
	t := newTable(opts)
	var r Result

	for i := 0; i < len(args); i++ {
		arg := args[i]
		switch {
		case arg == "--":
			r.Leftover = append(r.Leftover, args[i+1:]...)
			return r
		case strings.HasPrefix(arg, "--"):
			i += parseLong(t, &r, arg[2:], args, i)
		case len(arg) > 1 && arg[0] == '-':
			i += parseBundle(t, &r, arg[1:], args, i)
		default:
			// "-" and "" are positionals, not options.
			r.Events = append(r.Events, Event{Arg: arg})
		}
	}
	return r
}

// parseLong handles "--name", "--name=value" and "--name value", returning how
// many extra arguments it consumed.
func parseLong(t *table, r *Result, body string, args []string, i int) int {
	typed, value, hasValue := body, "", false
	if eq := strings.IndexByte(body, '='); eq >= 0 {
		typed, value, hasValue = body[:eq], body[eq+1:], true
	}

	key, opt, errMsg := t.resolve(typed)
	if errMsg != "" {
		r.Errors = append(r.Errors, errMsg)
		return 0
	}

	switch opt.Arg {
	case NoArg:
		if hasValue {
			r.Errors = append(r.Errors, fmt.Sprintf("Option %s does not take an argument", key))
			return 0
		}
		r.Events = append(r.Events, Event{Option: opt.Names[0]})
		return 0

	case StringArg:
		// An empty "--dir=" is a missing argument, not an empty string. A
		// separate next argument is taken verbatim, even "-v" or "--".
		if hasValue {
			if value == "" {
				r.Errors = append(r.Errors, fmt.Sprintf("Option %s requires an argument", key))
				return 0
			}
			r.Events = append(r.Events, Event{Option: opt.Names[0], Value: value, HasValue: true})
			return 0
		}
		if i+1 >= len(args) {
			r.Errors = append(r.Errors, fmt.Sprintf("Option %s requires an argument", key))
			return 0
		}
		r.Events = append(r.Events, Event{Option: opt.Names[0], Value: args[i+1], HasValue: true})
		return 1

	case OptionalIntArg:
		// "--verbose=" is an *absent* value, so it increments rather than
		// erroring, unlike the StringArg case above.
		if hasValue && value != "" {
			if !intLiteral.MatchString(value) {
				// This wording is Getopt::Long's, not stow's, and it changed in
				// Getopt::Long 2.55 from "(number expected)". It therefore varies
				// with the installed Perl rather than with the pinned stow
				// version. Ledger PL-19: pin the current upstream wording.
				r.Errors = append(r.Errors,
					fmt.Sprintf("Value %q invalid for option %s (integer number expected)", value, key))
				return 0
			}
			r.Events = append(r.Events, Event{Option: opt.Names[0], Value: value, HasValue: true})
			return 0
		}
		if !hasValue && i+1 < len(args) && intLiteral.MatchString(args[i+1]) {
			r.Events = append(r.Events, Event{Option: opt.Names[0], Value: args[i+1], HasValue: true})
			return 1
		}
		r.Events = append(r.Events, Event{Option: opt.Names[0]})
		return 0
	}
	return 0
}

// parseBundle handles a run of short options after a single "-", returning how
// many extra arguments it consumed. An option that takes a value swallows the
// rest of the bundle as that value, so "-ttgt" is "-t tgt" and "-Dpkg" is "-D"
// followed by the bundle "pkg" — which is why stow reports "Unknown option: k".
func parseBundle(t *table, r *Result, body string, args []string, i int) int {
	for j := 0; j < len(body); j++ {
		c := body[j]
		opt, ok := t.byShort[c]
		if !ok {
			r.Errors = append(r.Errors, fmt.Sprintf("Unknown option: %c", c))
			continue
		}
		rest := body[j+1:]

		switch opt.Arg {
		case NoArg:
			r.Events = append(r.Events, Event{Option: opt.Names[0]})

		case StringArg:
			if rest != "" {
				r.Events = append(r.Events, Event{Option: opt.Names[0], Value: rest, HasValue: true})
				return 0
			}
			if i+1 >= len(args) {
				r.Errors = append(r.Errors, fmt.Sprintf("Option %c requires an argument", c))
				return 0
			}
			r.Events = append(r.Events, Event{Option: opt.Names[0], Value: args[i+1], HasValue: true})
			return 1

		case OptionalIntArg:
			// The remainder is the value only if it is an integer; otherwise it
			// is more bundled options. So "-v3" sets 3, "-vv" increments twice,
			// and "-v=3" increments once and then chokes on '=' and '3'.
			if intLiteral.MatchString(rest) {
				r.Events = append(r.Events, Event{Option: opt.Names[0], Value: rest, HasValue: true})
				return 0
			}
			if rest == "" && i+1 < len(args) && intLiteral.MatchString(args[i+1]) {
				r.Events = append(r.Events, Event{Option: opt.Names[0], Value: args[i+1], HasValue: true})
				return 1
			}
			r.Events = append(r.Events, Event{Option: opt.Names[0]})
		}
	}
	return 0
}
