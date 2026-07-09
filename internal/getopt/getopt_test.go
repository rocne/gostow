package getopt

import (
	"strings"
	"testing"
)

// stowSpec mirrors bin/stow's GetOptionsFromArray call verbatim. Canonical names
// are the first of each group, matching the keys Getopt::Long writes into
// %options ("simulate", not "n"; "S", not "stow").
func stowSpec() []Option {
	return []Option{
		{Names: []string{"verbose", "v"}, Arg: OptionalIntArg},
		{Names: []string{"help", "h"}},
		{Names: []string{"simulate", "n", "no"}},
		{Names: []string{"version", "V"}},
		{Names: []string{"compat", "p"}},
		{Names: []string{"dir", "d"}, Arg: StringArg},
		{Names: []string{"target", "t"}, Arg: StringArg},
		{Names: []string{"adopt"}},
		{Names: []string{"no-folding"}},
		{Names: []string{"dotfiles"}},
		{Names: []string{"ignore"}, Arg: StringArg},
		{Names: []string{"override"}, Arg: StringArg},
		{Names: []string{"defer"}, Arg: StringArg},
		{Names: []string{"D", "delete"}},
		{Names: []string{"S", "stow"}},
		{Names: []string{"R", "restow"}},
	}
}

// render flattens a Result into one comparable line:
//
//	events | errors | leftover
//
// where an event is "arg", "--name", or "--name=value".
func render(r Result) string {
	var ev []string
	for _, e := range r.Events {
		switch {
		case e.Option == "":
			ev = append(ev, e.Arg)
		case e.HasValue:
			ev = append(ev, "--"+e.Option+"="+e.Value)
		default:
			ev = append(ev, "--"+e.Option)
		}
	}
	return strings.Join(ev, " ") + " | " + strings.Join(r.Errors, "; ") + " | " + strings.Join(r.Leftover, ",")
}

// Every expectation below was produced by running the real Getopt::Long with
// stow's exact configuration ('no_ignore_case', 'bundling', 'permute') and
// stow's exact option spec. They are recordings, not predictions.
func TestParseMatchesGetoptLong(t *testing.T) {
	tests := []struct {
		args []string
		want string
	}{
		// --- positionals and action switches -----------------------------
		{[]string{"pkg"}, "pkg |  | "},
		{[]string{"-D", "a", "-S", "b", "-R", "c"}, "--D a --S b --R c |  | "},
		{[]string{"--R", "x"}, "--R x |  | "},
		{[]string{"-"}, "- |  | "}, // a lone dash is a package name

		// --- bundling ----------------------------------------------------
		{[]string{"-nv", "pkg"}, "--simulate --verbose pkg |  | "},
		{[]string{"-vvv", "pkg"}, "--verbose --verbose --verbose pkg |  | "},
		{[]string{"-ttgt", "pkg"}, "--target=tgt pkg |  | "},
		{[]string{"-t", "tgt", "pkg"}, "--target=tgt pkg |  | "},
		{[]string{"-tv", "pkg"}, "--target=v pkg |  | "},
		{[]string{"-ttgt", "-ddir", "pkg"}, "--target=tgt --dir=dir pkg |  | "},
		{[]string{"-nvd", "stow", "pkg"}, "--simulate --verbose --dir=stow pkg |  | "},
		{[]string{"-dir", "x"}, "--dir=ir x |  | "}, // -d swallows "ir"
		{[]string{"-d", "-v", "pkg"}, "--dir=-v pkg |  | "},

		// A value-taking option eats the rest of its bundle; what follows a
		// no-arg option is more bundled options, hence "k" and "g" below.
		{[]string{"-Dpkg"}, "--D --compat | Unknown option: k; Unknown option: g | "},
		{[]string{"-x", "pkg"}, "pkg | Unknown option: x | "},
		{[]string{"-vabc", "pkg"}, "--verbose pkg | Unknown option: a; Unknown option: b; Unknown option: c | "},

		// --- permute -----------------------------------------------------
		{[]string{"pkg", "-n", "-v"}, "pkg --simulate --verbose |  | "},

		// --- auto_abbrev, and exact-match-beats-prefix --------------------
		{[]string{"--targ=tgt", "pkg"}, "--target=tgt pkg |  | "},
		{[]string{"--dot", "pkg"}, "--dotfiles pkg |  | "},
		{[]string{"--do", "pkg"}, "--dotfiles pkg |  | "},
		{[]string{"--a", "pkg"}, "--adopt pkg |  | "},
		{[]string{"--si", "pkg"}, "--simulate pkg |  | "},
		{[]string{"--st", "pkg"}, "--S pkg |  | "},
		{[]string{"--no-f", "pkg"}, "--no-folding pkg |  | "},
		{[]string{"--no-", "pkg"}, "--no-folding pkg |  | "},
		// "--no" is the exact alias of --simulate, not a prefix of --no-folding.
		{[]string{"--no", "pkg"}, "--simulate pkg |  | "},
		// "--d" is the exact alias of --dir, not an ambiguous prefix of
		// dir/defer/delete/dotfiles -- and it therefore takes an argument.
		{[]string{"--d", "pkg"}, "--dir=pkg |  | "},
		{[]string{"--n", "pkg"}, "--simulate pkg |  | "},
		{[]string{"--v", "pkg"}, "--verbose pkg |  | "},
		{[]string{"--V"}, "--version |  | "},

		// --- ambiguity ---------------------------------------------------
		{[]string{"--ver", "pkg"}, "pkg | Option ver is ambiguous (verbose, version) | "},
		{[]string{"--s", "pkg"}, "pkg | Option s is ambiguous (simulate, stow) | "},
		{[]string{"--de", "pkg"}, "pkg | Option de is ambiguous (defer, delete) | "},
		{[]string{"--nofolding", "pkg"}, "pkg | Unknown option: nofolding | "},

		// --- no_ignore_case ----------------------------------------------
		{[]string{"-h", "-V"}, "--help --version |  | "},
		{[]string{"--help", "--version"}, "--help --version |  | "},

		// --- ':+' optional-integer semantics ------------------------------
		{[]string{"-v", "pkg"}, "--verbose pkg |  | "},
		{[]string{"-vv", "pkg"}, "--verbose --verbose pkg |  | "},
		{[]string{"-v3", "pkg"}, "--verbose=3 pkg |  | "},
		{[]string{"-v0", "pkg"}, "--verbose=0 pkg |  | "},
		{[]string{"-vv3", "pkg"}, "--verbose --verbose=3 pkg |  | "},
		{[]string{"-v-1", "pkg"}, "--verbose=-1 pkg |  | "},
		{[]string{"--verbose", "pkg"}, "--verbose pkg |  | "},
		{[]string{"--verbose", "3", "pkg"}, "--verbose=3 pkg |  | "},   // a numeric next arg is swallowed
		{[]string{"--verbose", "-1", "pkg"}, "--verbose=-1 pkg |  | "}, // even a negative one
		{[]string{"-v", "+2", "pkg"}, "--verbose=+2 pkg |  | "},
		{[]string{"--verbose=+2", "pkg"}, "--verbose=+2 pkg |  | "},
		{[]string{"--verbose=9", "pkg"}, "--verbose=9 pkg |  | "},
		{[]string{"-v", "-v", "-v", "pkg"}, "--verbose --verbose --verbose pkg |  | "},
		{[]string{"-vv", "--verbose=0", "pkg"}, "--verbose --verbose --verbose=0 pkg |  | "},
		{[]string{"-v", "--verbose=0", "-v", "pkg"}, "--verbose --verbose=0 --verbose pkg |  | "},
		// An empty value is an *absent* value here, so it increments...
		{[]string{"--verbose="}, "--verbose |  | "},
		// ...but "-v=3" cannot: "=3" is not an integer, so "=" and "3" are
		// read as further bundled options.
		{[]string{"-v=3", "pkg"}, "--verbose pkg | Unknown option: =; Unknown option: 3 | "},
		{[]string{"--verbose=abc", "pkg"}, `pkg | Value "abc" invalid for option verbose (integer number expected) | `},

		// --- '=s' requires a value; empty is missing, not empty -----------
		{[]string{"--dir="}, " | Option dir requires an argument | "},
		{[]string{"--ignore="}, " | Option ignore requires an argument | "},
		{[]string{"-t"}, " | Option t requires an argument | "},
		{[]string{"-d"}, " | Option d requires an argument | "},
		{[]string{"--d"}, " | Option d requires an argument | "},         // exact alias keeps the typed name
		{[]string{"--targ"}, " | Option target requires an argument | "}, // an abbreviation is canonicalised
		{[]string{"--tar"}, " | Option target requires an argument | "},
		{[]string{"--dir=x=y", "pkg"}, "--dir=x=y pkg |  | "}, // only the first '=' splits
		{[]string{"--dir", "--", "pkg"}, "--dir=-- pkg |  | "},
		{[]string{"--dir", "x", "--dir", "y", "pkg"}, "--dir=x --dir=y pkg |  | "},
		{[]string{"--dotfiles=1", "pkg"}, "pkg | Option dotfiles does not take an argument | "},

		// --- repeatable list options --------------------------------------
		{[]string{"--ignore=a", "--ignore=b", "pkg"}, "--ignore=a --ignore=b pkg |  | "},

		// --- "--" terminator: the rest is leftover, never a package -------
		// stow discards its leftover array, which is ledger PL-03.
		{[]string{"--", "pkg"}, " |  | pkg"},
		{[]string{"--", "-v", "pkg"}, " |  | -v,pkg"},
		{[]string{"-n", "--", "-v"}, "--simulate |  | -v"},
		{[]string{"pkg", "--"}, "pkg |  | "},
	}

	spec := stowSpec()
	for _, tt := range tests {
		t.Run(strings.Join(tt.args, " "), func(t *testing.T) {
			if got := render(Parse(spec, tt.args)); got != tt.want {
				t.Errorf("Parse(%q)\n got: %s\nwant: %s", tt.args, got, tt.want)
			}
		})
	}
}

func TestResultOK(t *testing.T) {
	spec := stowSpec()
	if !Parse(spec, []string{"pkg"}).OK() {
		t.Error("Parse of a clean argv should be OK")
	}
	if Parse(spec, []string{"-x"}).OK() {
		t.Error("Parse with an unknown option should not be OK")
	}
}
