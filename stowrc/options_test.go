package stowrc

import (
	"fmt"
	"strings"
	"testing"
)

// The RE2 divergence, pinned (ledger PL-22, docs/DIVERGENCES.md §4).
//
// Perl accepts lookaround and backreferences; RE2 accepts neither. Real stow runs
// with these patterns. gostow must reject them at parse time with a diagnostic
// naming the flag, rather than compile something subtly different and silently
// mismatch — and it must keep accepting everything RE2 does support, including
// the inline flags that look like the unsupported syntax.
func TestPerlOnlyRegexConstructsAreRejectedAtParseTime(t *testing.T) {
	rejected := []string{
		`x(?!y)`,  // negative lookahead
		`x(?=y)`,  // positive lookahead
		`(?<=x)y`, // lookbehind
		`(k)\1`,   // backreference
	}
	for _, pattern := range rejected {
		p := Parse([]string{"--ignore=" + pattern, "pkg"})
		if len(p.Errors) != 1 {
			t.Errorf("--ignore=%q produced %d diagnostics, want 1", pattern, len(p.Errors))
			continue
		}
		// The diagnostic quotes the pattern with %q, so a backslash arrives escaped.
		if !strings.Contains(p.Errors[0], "--ignore") || !strings.Contains(p.Errors[0], fmt.Sprintf("%q", pattern)) {
			t.Errorf("--ignore=%q diagnostic %q names neither the flag nor the pattern", pattern, p.Errors[0])
		}
	}

	accepted := []string{`(?i)keep`, `\.log`, `a|b`, `x*`, `^$`, `0`}
	for _, pattern := range accepted {
		if p := Parse([]string{"--ignore=" + pattern, "pkg"}); len(p.Errors) != 0 {
			t.Errorf("--ignore=%q was rejected: %v", pattern, p.Errors)
		}
	}
}

// OptionNames is the option table's public projection, and four external
// references (the man page, three completion scripts) are held to it by the
// docs tests in internal/cli. Pin its own shape here: canonical-first ordering
// is what those consumers key on.
func TestOptionNamesProjectsTheSpec(t *testing.T) {
	names := OptionNames()
	if len(names) != len(spec()) {
		t.Fatalf("OptionNames() has %d entries, spec() has %d", len(names), len(spec()))
	}
	for i, opt := range spec() {
		if strings.Join(names[i], ",") != strings.Join(opt.Names, ",") {
			t.Errorf("entry %d = %v, want %v", i, names[i], opt.Names)
		}
	}
	seen := map[string]bool{}
	for _, entry := range names {
		seen[entry[0]] = true
	}
	for _, canonical := range []string{"verbose", "dir", "target", "ignore", "D", "S", "R", "gostow-fix"} {
		if !seen[canonical] {
			t.Errorf("OptionNames() is missing the canonical name %q", canonical)
		}
	}
}
