package stowrc

import (
	"bufio"
	"errors"
	"fmt"
	"io"
	"strings"
)

// Tokens tokenizes one rc source into its option tokens, line by line, with
// Perl's shellwords. name stands in for the file path in the read-failure
// message, whose bytes are parity-pinned.
//
// A failed read is what stow reports as a failed close. Perl's readline
// poisons the handle, so `close` returns false and stow dies — which is how
// `.stowrc` being a *directory* (open(2) succeeds; the first read returns
// EISDIR) stops stow before it touches anything. Go's Close reports none of
// that, so the read error has to be consulted directly. Ignoring it once made
// gostow treat an unreadable rc file as an empty one and stow the package
// anyway: the same shape as the swallowed EISDIR that once disabled all
// ignoring.
//
// fixQuirks enables real comment syntax. Without it, '#' is an ordinary word
// character to shellwords, so `--ignore=x # note` yields three tokens; the last
// two are read as package names and rc package names are discarded — which is
// why .stowrc comments appear to work by accident (ledger PL-02).
func Tokens(r io.Reader, name string, fixQuirks bool) ([]string, error) {
	lines, readErr := readLines(r)
	if readErr != nil {
		return nil, &DieError{Msg: fmt.Sprintf("Could not close open file: %s", name)}
	}

	var tokens []string
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
	return tokens, nil
}

// readLines reads r the way Perl's `while (my $line = <$FILE>) { chomp $line; }`
// does. It uses a bufio.Reader rather than a bufio.Scanner because a Scanner
// fails on any line over 64 KiB, and Perl imposes no such limit: a long line in
// a config file must not be the difference between a working stow and a broken
// one.
func readLines(r io.Reader) ([]string, error) {
	var lines []string
	br := bufio.NewReader(r)
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
