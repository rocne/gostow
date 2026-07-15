package stowrc

import (
	"strings"
	"testing"
)

func TestShellwords(t *testing.T) {
	tests := []struct {
		in   string
		want []string
	}{
		{`--dir=a --target=b`, []string{"--dir=a", "--target=b"}},
		{`--dir='a b'`, []string{"--dir=a b"}},
		{`--dir="a b"`, []string{"--dir=a b"}},
		{`--dir=a\ b`, []string{"--dir=a b"}},
		// '#' is not a comment character: PL-02.
		{`--ignore=x # note`, []string{"--ignore=x", "#", "note"}},
		{``, nil},
		{`   `, nil},
	}
	for _, tt := range tests {
		got, err := shellwords(tt.in)
		if err != nil {
			t.Fatalf("shellwords(%q): %v", tt.in, err)
		}
		if strings.Join(got, "\x1f") != strings.Join(tt.want, "\x1f") {
			t.Errorf("shellwords(%q) = %q, want %q", tt.in, got, tt.want)
		}
	}
}

// An unmatched quote is a tokenization error, not a diagnostic: stow's
// shellwords returns nothing and stow dies, so the parse never happens.
func TestTokensRejectsAnUnmatchedQuote(t *testing.T) {
	for _, line := range []string{`--dir='a`, `--dir="a`} {
		if _, err := Tokens(strings.NewReader(line+"\n"), ".stowrc", false); err == nil {
			t.Errorf("Tokens(%q) returned no error", line)
		}
	}
}

// Perl imposes no line-length limit on a config file, and a bufio.Scanner errors
// past 64 KiB. A long line must be read, not silently truncated to end-of-file
// and not rejected.
func TestTokensReadsALineLongerThanAScannerBuffer(t *testing.T) {
	long := strings.Repeat("a", 100*1024)
	tokens, err := Tokens(strings.NewReader("--ignore="+long+"\n"), ".stowrc", false)
	if err != nil {
		t.Fatalf("Tokens: %v", err)
	}
	if len(tokens) != 1 || tokens[0] != "--ignore="+long {
		t.Errorf("got %d token(s), first of length %d; want one token of length %d",
			len(tokens), len(tokens[0]), len("--ignore=")+len(long))
	}
}

// fixQuirks is the PL-02 toggle: with it, '#' begins a comment and '\#' is a
// literal '#'; without it, everything after '#' is ordinary tokens.
func TestTokensFixQuirksTogglesCommentHandling(t *testing.T) {
	const line = `--ignore=keep # --ignore=drop` + "\n"

	quirky, err := Tokens(strings.NewReader(line), ".stowrc", false)
	if err != nil {
		t.Fatal(err)
	}
	if want := []string{"--ignore=keep", "#", "--ignore=drop"}; strings.Join(quirky, " ") != strings.Join(want, " ") {
		t.Errorf("without fixQuirks: tokens = %q, want %q", quirky, want)
	}

	fixed, err := Tokens(strings.NewReader(line), ".stowrc", true)
	if err != nil {
		t.Fatal(err)
	}
	if len(fixed) != 1 || fixed[0] != "--ignore=keep" {
		t.Errorf("with fixQuirks: tokens = %q, want [--ignore=keep]", fixed)
	}

	escaped, err := Tokens(strings.NewReader(`--ignore=a\#b`+"\n"), ".stowrc", true)
	if err != nil {
		t.Fatal(err)
	}
	if len(escaped) != 1 || escaped[0] != "--ignore=a#b" {
		t.Errorf(`with fixQuirks: '\#' tokens = %q, want [--ignore=a#b]`, escaped)
	}
}
