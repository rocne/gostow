package mangen

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// The one that matters: the DIVERGENCES section committed in man/gostow.8 must be
// exactly what Render produces from the current docs/DIVERGENCES.md. Edit the doc
// without running `go generate ./...` and this goes red, naming the fix. It is the
// same golden discipline the conformance suite uses — the committed artifact is
// checked against a fresh regeneration, so the two provably cannot drift.
func TestManPageIsInSyncWithDivergencesDoc(t *testing.T) {
	root := repoRootForTest(t)

	doc, err := os.ReadFile(filepath.Join(root, DocPath))
	if err != nil {
		t.Fatal(err)
	}
	man, err := os.ReadFile(filepath.Join(root, ManPath))
	if err != nil {
		t.Fatal(err)
	}

	want, err := Render(string(doc))
	if err != nil {
		t.Fatalf("rendering the divergences doc: %v", err)
	}
	got, err := GeneratedBlock(string(man))
	if err != nil {
		t.Fatal(err)
	}

	if got != want {
		t.Errorf("man/gostow.8's DIVERGENCES section is out of date with %s.\n"+
			"Run: go generate ./...\n\nfirst difference:\n%s", DocPath, firstDiff(want, got))
	}
}

func firstDiff(want, got string) string {
	w := strings.Split(want, "\n")
	g := strings.Split(got, "\n")
	for i := 0; i < len(w) || i < len(g); i++ {
		var wl, gl string
		if i < len(w) {
			wl = w[i]
		}
		if i < len(g) {
			gl = g[i]
		}
		if wl != gl {
			return "line " + itoa(i+1) + ":\n  want: " + wl + "\n  got:  " + gl
		}
	}
	return "(identical — length mismatch only)"
}

func itoa(n int) string {
	if n == 0 {
		return "0"
	}
	var b []byte
	for n > 0 {
		b = append([]byte{byte('0' + n%10)}, b...)
		n /= 10
	}
	return string(b)
}

func repoRootForTest(t *testing.T) string {
	t.Helper()
	cwd, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	root, err := RepoRoot(cwd)
	if err != nil {
		t.Fatal(err)
	}
	return root
}

// The converter's units. Each feeds a scrap of Markdown and pins the roff, so a
// regression in one construct is a local failure rather than a mystery in the
// 200-line golden above.

func TestInlineEmphasisAndCode(t *testing.T) {
	cases := map[string]string{
		"plain":                "plain",
		"**bold**":             `\fBbold\fP`,
		"*italic*":             `\fIitalic\fP`,
		"`code`":               `\fBcode\fP`,
		"a `--flag` here":      `a \fB\-\-flag\fP here`,
		"em — dash":            `em \(em dash`,
		"**bold with `code`**": `\fBbold with \fBcode\fP\fP`,
		`a \ backslash`:        `a \e backslash`,
		"trailing…":            "trailing...",
	}
	for in, want := range cases {
		if got := inline(in); got != want {
			t.Errorf("inline(%q) = %q, want %q", in, got, want)
		}
	}
}

// A literal '*' inside a code span (the doc really does write `*`) must not make
// the bold/italic matcher swallow the rest of the line. This is the case that
// forced .+? over [^*] in the emphasis patterns.
func TestInlineLiteralAsteriskInCode(t *testing.T) {
	got := inline("contains a `(`, `[`, `+`, `*`, or `?`.")
	if strings.Contains(got, "**") {
		t.Errorf("inline left a literal ** in the output: %q", got)
	}
	for _, want := range []string{`\fB(\fP`, `\fB*\fP`, `\fB?\fP`} {
		if !strings.Contains(got, want) {
			t.Errorf("inline(...) = %q, missing %q", got, want)
		}
	}
}

func TestToRoffHeadingDropsTheNumber(t *testing.T) {
	got, err := toRoff("## 3. Things gostow reproduces")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(got, ".SS Things gostow reproduces") {
		t.Errorf("heading not rendered as an unnumbered .SS: %q", got)
	}
}

func TestToRoffTable(t *testing.T) {
	md := "| GNU Stow does this | gostow does this |\n" +
		"|---|---|\n" +
		"| **Dies** on a `(`. | Runs fine. |"
	got, err := toRoff(md)
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{
		`\fBGNU Stow does this\fP: \fBDies\fP on a \fB(\fP.`,
		".br",
		`\fBgostow does this\fP: Runs fine.`,
	} {
		if !strings.Contains(got, want) {
			t.Errorf("table rendering missing %q in:\n%s", want, got)
		}
	}
}

func TestToRoffCodeBlockIsVerbatim(t *testing.T) {
	md := "text\n\n```\ncomplete -F _gostow stow\n```\n"
	got, err := toRoff(md)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(got, ".EX") || !strings.Contains(got, ".EE") {
		t.Errorf("code block not wrapped in .EX/.EE:\n%s", got)
	}
	// A hyphen stays ASCII minus (\-), and no emphasis is interpreted.
	if !strings.Contains(got, `complete \-F _gostow stow`) {
		t.Errorf("code block content not rendered verbatim:\n%s", got)
	}
}

func TestToRoffMultiParagraphListItem(t *testing.T) {
	md := "- first paragraph of the item\n" +
		"  wrapped onto a second source line\n\n" +
		"  a continuation paragraph under the bullet\n"
	got, err := toRoff(md)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(got, `.IP \(bu 3`) {
		t.Errorf("no bullet emitted:\n%s", got)
	}
	if !strings.Contains(got, "first paragraph of the item wrapped onto a second source line") {
		t.Errorf("wrapped continuation not joined:\n%s", got)
	}
	if !strings.Contains(got, ".RS 3") || !strings.Contains(got, "a continuation paragraph under the bullet") {
		t.Errorf("continuation paragraph not indented under the item:\n%s", got)
	}
}

func TestToRoffRejectsNonASCIIItCannotMap(t *testing.T) {
	if _, err := toRoff("a poundsign £ appears"); err == nil {
		t.Error("toRoff accepted an unmapped non-ASCII byte; it should refuse to emit roff groff would warn on")
	}
}

func TestExtractStopsAtTheFirstUnnumberedHeading(t *testing.T) {
	md := "# Title\n\nintro\n\n## 1. First\n\nbody\n\n## The convention\n\nnot imported\n"
	region, err := extractDivergences(md)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(region, "## 1. First") {
		t.Errorf("numbered section was excluded:\n%s", region)
	}
	if strings.Contains(region, "not imported") {
		t.Errorf("import ran past the first unnumbered heading:\n%s", region)
	}
}

func TestSpliceReplacesOnlyBetweenMarkers(t *testing.T) {
	man := "before\n" + BeginMarker + "\nstale\n" + EndMarker + "\nafter\n"
	got, err := Splice(man, "fresh")
	if err != nil {
		t.Fatal(err)
	}
	want := "before\n" + BeginMarker + "\nfresh\n" + EndMarker + "\nafter\n"
	if got != want {
		t.Errorf("Splice = %q, want %q", got, want)
	}
}

func TestSpliceNeedsBothMarkers(t *testing.T) {
	if _, err := Splice("no markers here", "x"); err == nil {
		t.Error("Splice accepted a man page with no markers")
	}
}
