package cli

import (
	"bufio"
	"errors"
	"fmt"
	"io"
	"os"
	"os/user"
	"path/filepath"
	"regexp"
	"strings"
)

// readStowrcTokens reads the rc files and returns their tokens concatenated.
//
// Ledger PL-01: the man page says the search order is current-directory then
// home. The code builds ('.stowrc') and then unshifts "$HOME/.stowrc", so the
// real order is **home first**. Both files are read and their tokens
// concatenated into one option array, so for scalar options the *last* wins —
// which means ./.stowrc overrides ~/.stowrc. gostow follows the code.
//
// A file that exists but is not readable is silently skipped: stow tests -r.
//
// fixQuirks enables real comment syntax. Without it, '#' is an ordinary word
// character to shellwords, so `--ignore=x # note` yields three tokens; the last
// two are read as package names and rc package names are discarded — which is
// why .stowrc comments appear to work by accident (ledger PL-02).
func readStowrcTokens(fixQuirks bool) ([]string, error) {
	var files []string
	if home, ok := os.LookupEnv("HOME"); ok {
		files = append(files, filepath.Join(home, ".stowrc"))
	}
	files = append(files, ".stowrc")

	var tokens []string
	for _, file := range files {
		f, err := os.Open(file)
		if err != nil {
			continue
		}
		lines, readErr := readLines(f)
		_ = f.Close()

		// A failed read is what stow reports as a failed close. Perl's readline
		// poisons the handle, so `close` returns false and stow dies — which is
		// how `.stowrc` being a *directory* (open(2) succeeds; the first read
		// returns EISDIR) stops stow before it touches anything. Go's Close
		// reports none of that, so the read error has to be consulted directly.
		// Ignoring it made gostow treat an unreadable rc file as an empty one and
		// stow the package anyway: the same shape as the swallowed EISDIR that
		// once disabled all ignoring.
		if readErr != nil {
			return nil, &dieError{msg: fmt.Sprintf("Could not close open file: %s", file)}
		}

		for _, line := range lines {
			if fixQuirks {
				line = stripComment(line)
			}
			words, err := shellwords(line)
			if err != nil {
				return nil, err
			}
			tokens = append(tokens, words...)
		}
	}
	return tokens, nil
}

// readLines reads f the way Perl's `while (my $line = <$FILE>) { chomp $line; }`
// does. It uses a bufio.Reader rather than a bufio.Scanner because a Scanner
// fails on any line over 64 KiB, and Perl imposes no such limit: a long line in
// a config file must not be the difference between a working stow and a broken
// one.
func readLines(f *os.File) ([]string, error) {
	var lines []string
	br := bufio.NewReader(f)
	for {
		line, err := br.ReadString('\n')
		if line != "" {
			lines = append(lines, strings.TrimSuffix(strings.TrimSuffix(line, "\n"), "\r"))
		}
		if err != nil {
			if errors.Is(err, io.EOF) {
				return lines, nil
			}
			return nil, err
		}
	}
}

// shellwords ports Perl's Text::ParseWords::shellwords: split on whitespace,
// honouring single quotes (literal), double quotes (backslash escapes honoured),
// and backslash escapes outside quotes.
//
// Ledger PL-02: '#' is not special here, so `--ignore=x # note` parses as three
// tokens. The latter two are read as package names, and rc package names are
// discarded — which is why comments in a .stowrc appear to work by accident.
func shellwords(line string) ([]string, error) {
	var words []string
	var cur strings.Builder
	inWord := false

	for i := 0; i < len(line); i++ {
		c := line[i]
		switch {
		case c == ' ' || c == '\t' || c == '\n' || c == '\r':
			if inWord {
				words = append(words, cur.String())
				cur.Reset()
				inWord = false
			}
		case c == '\'':
			inWord = true
			j := strings.IndexByte(line[i+1:], '\'')
			if j < 0 {
				return nil, fmt.Errorf("unmatched single quote in .stowrc: %s", line)
			}
			cur.WriteString(line[i+1 : i+1+j])
			i += j + 1
		case c == '"':
			inWord = true
			i++
			for i < len(line) && line[i] != '"' {
				if line[i] == '\\' && i+1 < len(line) {
					i++
				}
				cur.WriteByte(line[i])
				i++
			}
			if i >= len(line) {
				return nil, fmt.Errorf("unmatched double quote in .stowrc: %s", line)
			}
		case c == '\\' && i+1 < len(line):
			inWord = true
			i++
			cur.WriteByte(line[i])
		default:
			inWord = true
			cur.WriteByte(c)
		}
	}
	if inWord {
		words = append(words, cur.String())
	}
	return words, nil
}

// stripComment removes an unescaped '#' and everything after it.
func stripComment(line string) string {
	for i := 0; i < len(line); i++ {
		switch line[i] {
		case '\\':
			i++
		case '#':
			return line[:i]
		}
	}
	return line
}

var (
	reEnvBraced = regexp.MustCompile(`\$\{([\w\s]+)\}`)
	reEnvBare   = regexp.MustCompile(`\$(\w+)`)
	reTilde     = regexp.MustCompile(`^~([^/]*)`)
)

// expandFilepath applies stow's environment-variable and tilde expansion to a
// --dir or --target read from a .stowrc. source names the option for the error
// message, which is byte-exact.
func expandFilepath(path, source string) (string, error) {
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
		return "", &dieError{msg: fmt.Sprintf(
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
