// Package mangen imports docs/DIVERGENCES.md into the DIVERGENCES section of
// man/gostow.8, so the two cannot drift.
//
// The man page must not carry a hand-maintained second copy of the divergence
// list: edit one, forget the other, and the tool now lies about how it differs
// from GNU Stow. docs/DIVERGENCES.md is the single source of truth. This package
// converts the relevant slice of it to roff and splices it between two markers in
// the man page; `go generate ./...` runs the splice, and a test
// (mangen_test.go) fails if the committed man page is out of date.
//
// The converter handles exactly the Markdown that DIVERGENCES.md uses — headings,
// paragraphs, bold/italic/code spans, two-column tables, fenced code, and bullet
// lists with continuation paragraphs — and errors out on anything it does not
// recognise or on any byte it cannot render as ASCII, rather than emitting roff
// that groff will warn about. It is not a general Markdown engine and is not
// meant to become one.
package mangen

//go:generate go run ./cmd

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"
)

const (
	// BeginMarker and EndMarker bracket the generated region in man/gostow.8.
	// Everything between them is replaced wholesale; everything else is left
	// untouched.
	BeginMarker = `.\" BEGIN GENERATED DIVERGENCES`
	EndMarker   = `.\" END GENERATED DIVERGENCES`

	provenance = `.\" Imported from docs/DIVERGENCES.md by internal/mangen; do not edit here.
.\" Edit docs/DIVERGENCES.md and run: go generate ./...`
)

var (
	reHeading = regexp.MustCompile(`^##\s+(?:\d+\.\s+)?(.*\S)\s*$`)
	// A numbered section heading, "## 3. Title" — the boundary of the import.
	reNumberedHeading = regexp.MustCompile(`^##\s+\d+\.\s`)
	reBold            = regexp.MustCompile(`\*\*(.+?)\*\*`)
	reItalic          = regexp.MustCompile(`\*(.+?)\*`)
	reCode            = regexp.MustCompile("`(.+?)`")
	reTableSep        = regexp.MustCompile(`^\s*:?-+:?\s*$`)
)

// Render returns the roff that belongs between the markers: a provenance header
// followed by the converted divergence sections.
func Render(markdown string) (string, error) {
	region, err := extractDivergences(markdown)
	if err != nil {
		return "", err
	}
	body, err := toRoff(region)
	if err != nil {
		return "", err
	}
	return provenance + "\n" + body, nil
}

// extractDivergences returns the slice of the doc to import: everything from the
// intro paragraph after the single-# title down to (but not including) the first
// unnumbered "## " heading — i.e. the numbered divergence sections and nothing
// after them. Adding a "## 6." section extends the import; adding prose under a
// named heading like "## The --gostow- convention" does not.
func extractDivergences(markdown string) (string, error) {
	lines := strings.Split(markdown, "\n")

	start := -1
	for i, line := range lines {
		if strings.HasPrefix(line, "# ") {
			start = i + 1
			break
		}
	}
	if start == -1 {
		return "", errors.New("mangen: no '# ' title found in the divergences doc")
	}
	for start < len(lines) && strings.TrimSpace(lines[start]) == "" {
		start++
	}

	end := len(lines)
	for i := start; i < len(lines); i++ {
		if strings.HasPrefix(lines[i], "## ") && !reNumberedHeading.MatchString(lines[i]) {
			end = i
			break
		}
	}
	if end <= start {
		return "", errors.New("mangen: the divergences import region is empty")
	}
	return strings.Join(lines[start:end], "\n"), nil
}

type roffBuilder struct {
	out []string
	err error
}

func (b *roffBuilder) cmd(s string) { b.out = append(b.out, s) }

// text emits a content line, guarding a leading '.' or '\” — either would be
// read as a roff control line, and an ellipsis-opened sentence ("...followed by")
// or a table cell can produce one.
func (b *roffBuilder) text(s string) {
	if strings.HasPrefix(s, ".") || strings.HasPrefix(s, "'") {
		s = `\&` + s
	}
	b.out = append(b.out, s)
}

// toRoff converts the imported Markdown region. It is a line-oriented state
// machine because the region's blocks (headings, tables, fences, lists) are all
// distinguishable from their first line.
func toRoff(md string) (string, error) {
	lines := strings.Split(md, "\n")
	b := &roffBuilder{}

	var para []string
	flush := func() {
		if len(para) > 0 {
			b.cmd(".PP")
			b.text(inline(strings.Join(para, " ")))
			para = nil
		}
	}

	i := 0
	for i < len(lines) {
		line := lines[i]
		trimmed := strings.TrimSpace(line)

		switch {
		case strings.HasPrefix(trimmed, "```"):
			flush()
			i = emitCodeBlock(b, lines, i)

		case trimmed == "":
			flush()
			i++

		case trimmed == "---":
			flush()
			i++

		case strings.HasPrefix(line, "# ") && !strings.HasPrefix(line, "## "):
			flush()
			i++ // the title; our own .SH names the section

		case reHeading.MatchString(line):
			flush()
			b.cmd(".SS " + inline(reHeading.FindStringSubmatch(line)[1]))
			i++

		case strings.HasPrefix(trimmed, "|"):
			flush()
			i = emitTable(b, lines, i)

		case strings.HasPrefix(trimmed, "- "):
			flush()
			i = emitListItem(b, lines, i)

		default:
			para = append(para, trimmed)
			i++
		}
	}
	flush()

	if b.err != nil {
		return "", b.err
	}
	result := strings.Join(b.out, "\n")
	if bad := firstNonASCII(result); bad != "" {
		return "", fmt.Errorf("mangen: produced a non-ASCII byte near %q; add a mapping to inline()", bad)
	}
	return result, nil
}

func isFence(line string) bool { return strings.HasPrefix(strings.TrimSpace(line), "```") }

// emitCodeBlock renders a fenced block verbatim in a constant-width, no-fill
// region. i points at the opening fence; the returned index is past the close.
func emitCodeBlock(b *roffBuilder, lines []string, i int) int {
	i++ // opening fence
	// .EX/.EE is the man(7) example idiom: no-fill, constant-width where the
	// device has it, and — unlike an explicit `.ft CW` — no "cannot select font"
	// warning on the utf8 (terminal) device, which has no CW font to select.
	b.cmd(".PP")
	b.cmd(".RS 4")
	b.cmd(".EX")
	for i < len(lines) && !isFence(lines[i]) {
		b.text(codeEscape(lines[i]))
		i++
	}
	b.cmd(".EE")
	b.cmd(".RE")
	if i < len(lines) {
		i++ // closing fence
	}
	return i
}

// emitTable renders a two-column "GNU Stow does this / gostow does this" table.
// Each row becomes a pair of labelled paragraphs rather than a roff table: tbl
// would need a preprocessor line and wrapping-cell blocks, and the labelled pair
// reads well and stays trivially ASCII-clean.
func emitTable(b *roffBuilder, lines []string, i int) int {
	var rows [][]string
	for i < len(lines) && strings.HasPrefix(strings.TrimSpace(lines[i]), "|") {
		rows = append(rows, splitRow(lines[i]))
		i++
	}
	if len(rows) < 2 {
		b.err = errors.New("mangen: a table needs a header row and at least one body row")
		return i
	}
	header := rows[0]
	if len(header) != 2 {
		b.err = fmt.Errorf("mangen: only two-column tables are supported, got %d columns", len(header))
		return i
	}

	for _, row := range rows[1:] {
		if isSeparatorRow(row) {
			continue
		}
		if len(row) != 2 {
			b.err = fmt.Errorf("mangen: table row has %d columns, want 2: %v", len(row), row)
			return i
		}
		b.cmd(".PP")
		b.text(`\fB` + inline(header[0]) + `\fP: ` + inline(row[0]))
		b.cmd(".br")
		b.text(`\fB` + inline(header[1]) + `\fP: ` + inline(row[1]))
	}
	return i
}

func splitRow(line string) []string {
	line = strings.TrimSpace(line)
	line = strings.TrimPrefix(line, "|")
	line = strings.TrimSuffix(line, "|")
	parts := strings.Split(line, "|")
	for j := range parts {
		parts[j] = strings.TrimSpace(parts[j])
	}
	return parts
}

func isSeparatorRow(cells []string) bool {
	for _, c := range cells {
		if !reTableSep.MatchString(c) {
			return false
		}
	}
	return true
}

// emitListItem renders one bullet and any continuation paragraphs indented under
// it. i points at the "- " line; the returned index is the next unconsumed line.
func emitListItem(b *roffBuilder, lines []string, i int) int {
	var paras [][]string
	cur := []string{strings.TrimSpace(lines[i])[2:]}
	i++

	for i < len(lines) {
		line := lines[i]
		if strings.TrimSpace(line) == "" {
			// A blank line ends the item unless the next non-blank line is an
			// indented continuation paragraph belonging to it.
			k := i + 1
			for k < len(lines) && strings.TrimSpace(lines[k]) == "" {
				k++
			}
			if k < len(lines) && strings.HasPrefix(lines[k], " ") && !strings.HasPrefix(strings.TrimSpace(lines[k]), "- ") {
				paras = append(paras, cur)
				cur = nil
				i = k
				continue
			}
			break
		}
		if strings.HasPrefix(line, " ") { // a wrapped continuation of the current paragraph
			cur = append(cur, strings.TrimSpace(line))
			i++
			continue
		}
		break // a dedented paragraph, or the next "- " item
	}
	paras = append(paras, cur)

	b.cmd(`.IP \(bu 3`)
	b.text(inline(strings.Join(paras[0], " ")))
	if len(paras) > 1 {
		b.cmd(".RS 3")
		for _, p := range paras[1:] {
			b.cmd(".PP")
			b.text(inline(strings.Join(p, " ")))
		}
		b.cmd(".RE")
	}
	return i
}

// inline converts a run of text: emphasis and code spans to font escapes, the
// Unicode punctuation the doc uses to roff escapes, and hyphens to \- so groff
// neither hyphenates nor typesets them as anything but ASCII minus.
//
// Order matters. Backslashes are escaped first, before any are introduced.
// Emphasis runs before code so that bold-wrapping a cell that contains a code
// span nests cleanly (roff's \fP restores the previous font). Hyphens run last,
// after every escape that might contain one — none do, which is what makes the
// blanket replacement safe.
func inline(s string) string {
	s = strings.ReplaceAll(s, `\`, `\e`)
	s = strings.ReplaceAll(s, "—", `\(em`)
	s = strings.ReplaceAll(s, "–", `\(en`)
	s = strings.ReplaceAll(s, "…", "...")
	s = reBold.ReplaceAllString(s, `\fB$1\fP`)
	s = reItalic.ReplaceAllString(s, `\fI$1\fP`)
	s = reCode.ReplaceAllString(s, `\fB$1\fP`)
	s = strings.ReplaceAll(s, "-", `\-`)
	return s
}

// codeEscape renders a verbatim line: a literal backslash becomes \e, a hyphen
// \-, and the same Unicode punctuation is mapped, but no Markdown is interpreted.
func codeEscape(s string) string {
	s = strings.ReplaceAll(s, `\`, `\e`)
	s = strings.ReplaceAll(s, "—", `\(em`)
	s = strings.ReplaceAll(s, "–", `\(en`)
	s = strings.ReplaceAll(s, "…", "...")
	s = strings.ReplaceAll(s, "-", `\-`)
	return s
}

func firstNonASCII(s string) string {
	for i, r := range s {
		if r > 127 {
			lo := i - 10
			if lo < 0 {
				lo = 0
			}
			hi := i + 10
			if hi > len(s) {
				hi = len(s)
			}
			return s[lo:hi]
		}
	}
	return ""
}

// Splice replaces the region between the markers in the man page with body,
// leaving the markers and everything outside them intact.
func Splice(manpage, body string) (string, error) {
	lines := strings.Split(manpage, "\n")
	begin, end := -1, -1
	for i, line := range lines {
		switch strings.TrimSpace(line) {
		case BeginMarker:
			begin = i
		case EndMarker:
			end = i
		}
	}
	if begin == -1 || end == -1 {
		return "", fmt.Errorf("mangen: man page is missing the %q / %q markers", BeginMarker, EndMarker)
	}
	if end < begin {
		return "", errors.New("mangen: END marker precedes BEGIN marker")
	}

	var next []string
	next = append(next, lines[:begin+1]...)
	next = append(next, strings.Split(body, "\n")...)
	next = append(next, lines[end:]...)
	return strings.Join(next, "\n"), nil
}

// GeneratedBlock returns the text currently sitting between the markers, so a
// test can compare it against a fresh Render without rewriting anything.
func GeneratedBlock(manpage string) (string, error) {
	lines := strings.Split(manpage, "\n")
	begin, end := -1, -1
	for i, line := range lines {
		switch strings.TrimSpace(line) {
		case BeginMarker:
			begin = i
		case EndMarker:
			end = i
		}
	}
	if begin == -1 || end == -1 {
		return "", fmt.Errorf("mangen: man page is missing the %q / %q markers", BeginMarker, EndMarker)
	}
	return strings.Join(lines[begin+1:end], "\n"), nil
}

// RepoRoot walks up from dir to the module root.
func RepoRoot(dir string) (string, error) {
	for {
		if _, err := os.Stat(filepath.Join(dir, "go.mod")); err == nil {
			return dir, nil
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			return "", errors.New("mangen: go.mod not found above " + dir)
		}
		dir = parent
	}
}

// DocPath and ManPath name the two files, relative to the repo root.
const (
	DocPath = "docs/DIVERGENCES.md"
	ManPath = "man/gostow.8"
)
