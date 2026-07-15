package stowrc

import (
	"fmt"
	"os"
	"os/user"
	"regexp"
	"strings"
)

var (
	reEnvBraced = regexp.MustCompile(`\$\{([\w\s]+)\}`)
	reEnvBare   = regexp.MustCompile(`\$(\w+)`)
	reTilde     = regexp.MustCompile(`^~([^/]*)`)
)

// ExpandFilepath applies stow's environment-variable and tilde expansion to a
// --dir or --target read from a .stowrc. source names the option for the error
// message, which is byte-exact ("--dir option" / "--target option").
// [ParseFile] and [ParseReader] apply it already; it is exported for callers
// parsing at the token level, who must apply it post-parse as stow does.
func ExpandFilepath(path, source string) (string, error) {
	expanded, err := expandEnvironment(path, source)
	if err != nil {
		return "", err
	}
	return expandTilde(expanded), nil
}

// expandEnvironment replaces $VAR and ${VAR} unless preceded by a backslash, and
// then unescapes \$. An undefined variable is fatal — stow refuses to guess.
func expandEnvironment(path, source string) (string, error) {
	var failed string
	// Perl guards each substitution with the lookbehind (?<!\\), which RE2
	// cannot express, so the escape check is done against the match offset.
	expand := func(re *regexp.Regexp, s string) string {
		var b strings.Builder
		last := 0
		for _, loc := range re.FindAllStringSubmatchIndex(s, -1) {
			start, end := loc[0], loc[1]
			if start > 0 && s[start-1] == '\\' {
				continue
			}
			name := s[loc[2]:loc[3]]
			val, ok := os.LookupEnv(name)
			if !ok && failed == "" {
				failed = name
			}
			b.WriteString(s[last:start])
			b.WriteString(val)
			last = end
		}
		b.WriteString(s[last:])
		return b.String()
	}
	path = expand(reEnvBraced, path)
	path = expand(reEnvBare, path)
	if failed != "" {
		return "", &DieError{Msg: fmt.Sprintf(
			"%s references undefined environment variable $%s; aborting!", source, failed)}
	}
	return strings.ReplaceAll(path, `\$`, "$"), nil
}

func expandTilde(path string) string {
	path = reTilde.ReplaceAllStringFunc(path, func(m string) string {
		name := m[1:]
		if name == "" {
			if home, ok := os.LookupEnv("HOME"); ok && home != "" {
				return home
			}
			if logdir, ok := os.LookupEnv("LOGDIR"); ok && logdir != "" {
				return logdir
			}
			if u, err := user.Current(); err == nil {
				return u.HomeDir
			}
			return m
		}
		// An unknown user leaves the token alone, which the caller's "is not a
		// valid directory" check then rejects.
		//
		// stow does not. Perl's (getpwnam($1))[7] is undef, it interpolates as the
		// empty string, and `--target=~nosuchuser/tmp/x` becomes `/tmp/x` — a
		// directory the user never named, which stow will happily build a symlink
		// farm inside, exit 0, with nothing but a `Use of uninitialized value`
		// warning to show for it. That is an uninitialized value reaching a
		// filesystem operation, so it is a bug rather than behaviour, and the
		// parity mandate exempts stow's bugs. Ledger PL-21; owed upstream.
		if u, err := user.Lookup(name); err == nil {
			return u.HomeDir
		}
		return m
	})
	return strings.ReplaceAll(path, `\~`, "~")
}
