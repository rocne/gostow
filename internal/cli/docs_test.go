package cli

import (
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"testing"
)

// spec() is the only place gostow's options are declared. The man page and the
// three completion scripts are four more places a user reads them from, and
// nothing but a test keeps those four in step: a flag added to spec() is silently
// absent from `gostow --dir <TAB>` forever, and a flag typo'd in a completion
// script silently completes nothing.
//
// So every one of them is required to name *exactly* the options gostow accepts.
// Equality, not containment, in both directions — containment would let a typo
// like --dotfile sit in a completion script next to the real --dotfiles.
//
// GNU Stow's own two references disagree with each other, which is what made this
// worth pinning: `stow --help` never mentions --no-folding, and stow.8 never
// mentions --compat/-p. Neither document lists every option stow accepts. See
// docs/SPEC.md §10.

// specFlags renders spec() as the flag strings a user would type.
func specFlags(t *testing.T) []string {
	t.Helper()
	var out []string
	for _, opt := range spec() {
		for _, name := range opt.Names {
			if len(name) == 1 {
				out = append(out, "-"+name)
			} else {
				out = append(out, "--"+name)
			}
		}
	}
	if len(out) < 20 {
		t.Fatalf("spec() rendered only %d flags; the renderer is broken and every test below would be vacuous", len(out))
	}
	sort.Strings(out)
	return out
}

func repoFile(t *testing.T, rel string) string {
	t.Helper()
	dir, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	for {
		if _, err := os.Stat(filepath.Join(dir, "go.mod")); err == nil {
			break
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			t.Fatal("go.mod not found above the working directory")
		}
		dir = parent
	}
	path := filepath.Join(dir, rel)
	b, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("reading %s: %v", rel, err)
	}
	return string(b)
}

var reLong = regexp.MustCompile(`--([a-z][a-z-]*)`)

func isAlpha(b byte) bool { return (b >= 'a' && b <= 'z') || (b >= 'A' && b <= 'Z') }

func isWordByte(b byte) bool {
	return b == '_' || b == '-' || (b >= '0' && b <= '9') || isAlpha(b)
}

// flagsIn extracts every option-looking token from plain text. Callers must feed
// it text where such a token can only be an option, or the shorts pick up prose.
//
// The short scan is a hand-rolled walk rather than a regexp because a regexp's
// matches cannot overlap: a pattern that requires a delimiter on each side of
// "-v" consumes the space before "-V", and `-v -V` yields only the first. That
// bug would have quietly dropped half the short options from every assertion
// below, and left the tests passing.
func flagsIn(text string) []string {
	seen := map[string]bool{}
	for _, m := range reLong.FindAllStringSubmatch(text, -1) {
		seen["--"+m[1]] = true
	}
	for i := 0; i+1 < len(text); i++ {
		if text[i] != '-' {
			continue
		}
		if i > 0 && isWordByte(text[i-1]) {
			continue // "--dir", or a hyphenated word like "drop-in"
		}
		if !isAlpha(text[i+1]) {
			continue
		}
		if i+2 < len(text) && isWordByte(text[i+2]) {
			continue // "-dir" is not the short option "-d"
		}
		seen["-"+string(text[i+1])] = true
	}
	out := make([]string, 0, len(seen))
	for f := range seen {
		out = append(out, f)
	}
	sort.Strings(out)
	return out
}

// unroff turns man(7) source into the text a reader sees, well enough to find
// option names in it: font selections vanish, "\-" is a literal hyphen, and the
// special characters this page uses collapse to a space.
func unroff(src string) string {
	r := strings.NewReplacer(
		`\f(CW`, "", `\fB`, "", `\fI`, "", `\fR`, "", `\fP`, "",
		`\(lq`, " ", `\(rq`, " ", `\(em`, " ", `\(bu`, " ", `\(co`, " ",
		`\&`, "", `\e`, `\`,
		`\-`, "-",
	)
	var lines []string
	for _, line := range strings.Split(src, "\n") {
		if strings.HasPrefix(line, `.\"`) {
			continue // a roff comment; the header explains the design in one
		}
		lines = append(lines, r.Replace(line))
	}
	return strings.Join(lines, "\n")
}

func assertSameFlags(t *testing.T, what string, want, got []string) {
	t.Helper()
	if len(got) == 0 {
		t.Fatalf("%s: extracted no flags at all; the extractor is broken and this test would be vacuous", what)
	}
	wantSet := map[string]bool{}
	for _, f := range want {
		wantSet[f] = true
	}
	gotSet := map[string]bool{}
	for _, f := range got {
		gotSet[f] = true
	}
	for _, f := range want {
		if !gotSet[f] {
			t.Errorf("%s: gostow accepts %s, but %s never names it", what, f, what)
		}
	}
	for _, f := range got {
		if !wantSet[f] {
			t.Errorf("%s: names %s, which gostow does not accept (typo, or a flag that was removed)", what, f)
		}
	}
	if !t.Failed() {
		t.Logf("%s: %d options, matching spec() exactly", what, len(got))
	}
}

func TestManPageDocumentsExactlyTheOptionsGostowAccepts(t *testing.T) {
	src := repoFile(t, "man/gostow.8")
	assertSameFlags(t, "man/gostow.8", specFlags(t), flagsIn(unroff(src)))
}

// stripComments removes whole-line comments so a completion script's own prose
// cannot satisfy a test about its code. Each of the three files documents, in a
// comment, the one-liner a drop-in user adds to complete `stow` — the very text
// TestCompletionsDoNotClaimTheNameStow is looking for.
func stripComments(src string) string {
	var lines []string
	for _, line := range strings.Split(src, "\n") {
		if strings.HasPrefix(strings.TrimSpace(line), "#") {
			continue
		}
		lines = append(lines, line)
	}
	return strings.Join(lines, "\n")
}

// bashFlags reads the single `flags='...'` assignment. The completion offers
// exactly that word list, so that list is what the test must check.
func bashFlags(t *testing.T, src string) []string {
	t.Helper()
	m := regexp.MustCompile(`(?s)flags='([^']*)'`).FindStringSubmatch(src)
	if m == nil {
		t.Fatal("completions/gostow.bash: no flags='...' assignment found; the extractor is broken")
	}
	out := strings.Fields(m[1])
	sort.Strings(out)
	return out
}

// zshFlags reads the option specs handed to _arguments. Only lines that begin a
// spec are scanned: `_arguments -s -S` is an invocation, and its own -s and -S
// are not gostow's.
func zshFlags(t *testing.T, src string) []string {
	t.Helper()
	var specLines []string
	for _, line := range strings.Split(src, "\n") {
		trimmed := strings.TrimSpace(line)
		if strings.HasPrefix(trimmed, "'") {
			specLines = append(specLines, trimmed)
		}
	}
	if len(specLines) == 0 {
		t.Fatal("completions/_gostow: no _arguments specs found; the extractor is broken")
	}
	// A spec's description text sits inside [...]; strip it so prose like
	// "(0-5)" or a hyphenated word cannot masquerade as a short option.
	desc := regexp.MustCompile(`\[[^\]]*\]`)
	joined := desc.ReplaceAllString(strings.Join(specLines, "\n"), " ")
	return flagsIn(joined)
}

// completeLines returns the lines that invoke the shell's completion builtin.
func completeLines(src, builtin string) []string {
	var out []string
	for _, line := range strings.Split(src, "\n") {
		if fields := strings.Fields(line); len(fields) > 0 && fields[0] == builtin {
			out = append(out, line)
		}
	}
	return out
}

// fishFlags reads fish's own declarations: `-s x` is a short option and `-l name`
// a long one. This file uses --description rather than fish's -d for the help
// text, so nothing else on a `complete` line can be mistaken for a gostow flag.
//
// Only `complete` lines are scanned, and that is not a tidiness preference. fish
// spells "declare a local variable" as `set -l`, so the helper functions above
// contain `set -l tokens`, `set -l n`, `set -l next` — which a whole-file scan
// reads as the long options --tokens, --n and --next. It did, the first time.
func fishFlags(t *testing.T, src string) []string {
	t.Helper()
	seen := map[string]bool{}
	joined := strings.Join(completeLines(src, "complete"), "\n")
	for _, m := range regexp.MustCompile(`(^|\s)-s\s+([A-Za-z])(\s|$)`).FindAllStringSubmatch(joined, -1) {
		seen["-"+m[2]] = true
	}
	for _, m := range regexp.MustCompile(`(^|\s)-l\s+([a-z][a-z-]*)`).FindAllStringSubmatch(joined, -1) {
		seen["--"+m[2]] = true
	}
	if len(seen) == 0 {
		t.Fatal("completions/gostow.fish: no -s/-l declarations found; the extractor is broken")
	}
	out := make([]string, 0, len(seen))
	for f := range seen {
		out = append(out, f)
	}
	sort.Strings(out)
	return out
}

func TestCompletionsOfferExactlyTheOptionsGostowAccepts(t *testing.T) {
	want := specFlags(t)

	t.Run("bash", func(t *testing.T) {
		src := stripComments(repoFile(t, "completions/gostow.bash"))
		assertSameFlags(t, "completions/gostow.bash", want, bashFlags(t, src))
	})
	t.Run("zsh", func(t *testing.T) {
		src := stripComments(repoFile(t, "completions/_gostow"))
		assertSameFlags(t, "completions/_gostow", want, zshFlags(t, src))
	})
	t.Run("fish", func(t *testing.T) {
		src := stripComments(repoFile(t, "completions/gostow.fish"))
		assertSameFlags(t, "completions/gostow.fish", want, fishFlags(t, src))
	})
}

// gostow completes `gostow`, and not `stow`.
//
// In bash and fish the filename is the lookup key — the loader opens a file named
// after the command being completed — so a `stow` registration inside gostow's own
// file is dead code, and making it live means shipping a file at the path `stow`,
// which belongs to GNU Stow in every package namespace. zsh would allow it (its
// #compdef tag names commands, not the file), but a completion that appears in one
// shell and not the other two is worse than one that appears in none.
//
// .goreleaser/gostow.yaml already declines to claim the name `stow` for the
// package. This declines it for the completions, and pins that decision so a
// later "helpful" edit has to argue with a test.
// The test asks which *commands* each file registers, which is a different
// question from which words it contains. Scanning for the bare word "stow"
// reports zsh's description text `[set the stow directory]`, and fish's `-l stow`
// — which is how fish spells the perfectly legitimate option --stow.
func TestCompletionsDoNotClaimTheNameStow(t *testing.T) {
	assertRegisters := func(file string, got []string) {
		t.Helper()
		if len(got) == 0 {
			t.Fatalf("%s: registers no command at all; the extractor is broken and this test would be vacuous", file)
		}
		for _, cmd := range got {
			if cmd != "gostow" {
				t.Errorf("%s: registers the command %q; see this test's comment for why it must not", file, cmd)
			}
		}
	}

	// bash: `complete [options] NAME...` — the trailing words are command names.
	bash := stripComments(repoFile(t, "completions/gostow.bash"))
	var bashCmds []string
	for _, line := range completeLines(bash, "complete") {
		fields := strings.Fields(line)
		for i := 1; i < len(fields); i++ {
			switch fields[i] {
			case "-F", "-C", "-W", "-o", "-A", "-G", "-X", "-P", "-S":
				i++ // an option that takes an argument; skip both
			default:
				if !strings.HasPrefix(fields[i], "-") {
					bashCmds = append(bashCmds, fields[i])
				}
			}
		}
	}
	assertRegisters("completions/gostow.bash", bashCmds)

	// zsh: the #compdef tag names the commands, not the filename — which is why
	// zsh alone *could* have claimed `stow` from inside this file.
	zsh := repoFile(t, "completions/_gostow")
	first, _, _ := strings.Cut(zsh, "\n")
	tag, ok := strings.CutPrefix(first, "#compdef ")
	if !ok {
		t.Fatalf("completions/_gostow: first line is %q, not a #compdef tag", first)
	}
	assertRegisters("completions/_gostow", strings.Fields(tag))

	// fish: `complete -c NAME` / `complete --command NAME`.
	fish := stripComments(repoFile(t, "completions/gostow.fish"))
	var fishCmds []string
	for _, line := range completeLines(fish, "complete") {
		fields := strings.Fields(line)
		for i := 0; i+1 < len(fields); i++ {
			if fields[i] == "-c" || fields[i] == "--command" {
				fishCmds = append(fishCmds, fields[i+1])
			}
		}
	}
	assertRegisters("completions/gostow.fish", fishCmds)
}

// The opt-in one-liner is the only thing a drop-in user has to go on, so it must
// actually be there. A comment is not usually worth a test; this one is load
// bearing, because TestCompletionsDoNotClaimTheNameStow removes the alternative.
func TestCompletionsDocumentTheDropInOptIn(t *testing.T) {
	for f, want := range map[string]string{
		"completions/gostow.bash": "complete -F _gostow stow",
		"completions/_gostow":     "compdef _gostow stow",
		"completions/gostow.fish": "complete -c stow --wraps gostow",
	} {
		if !strings.Contains(repoFile(t, f), want) {
			t.Errorf("%s: does not show the drop-in user how to opt in (%q)", f, want)
		}
	}
}
