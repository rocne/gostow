// Package ui is gostow's sole additive liberty over GNU Stow: colour on a TTY.
//
// The mandate is byte-compatible on a pipe, prettier on a TTY (CLAUDE.md; SPEC
// §8.4). Two structural choices make that a property of the code rather than a
// promise in a document:
//
//  1. Colour is a *rendering pass over finished lines*, not a set of coloured
//     format strings at the call sites. A rule may only wrap an existing
//     substring in SGR escapes; the painted line is reassembled from the
//     original bytes. Stripping the escapes therefore returns the input exactly,
//     and color_test.go asserts that over every line shape gostow emits.
//
//  2. When colour is off, the rendering pass does not run at all: Writer hands
//     the bytes to the underlying stream untouched. Code that never executes
//     cannot perturb a pipe.
//
// The engine (package stow) writes plain text to an io.Writer and knows nothing
// about terminals. The CLI hands it a Writer from here. That is the whole of
// gostow's output styling, in one place.
package ui

import (
	"io"
	"os"
	"regexp"
	"strings"
)

// SGR escape sequences. Only colour and intensity are used: nothing here moves
// the cursor, clears a line, or otherwise assumes an addressable screen, so
// output remains meaningful when captured with `script` or piped into `less -R`.
const (
	sgrReset = "\x1b[0m"

	bold    = "1"
	dim     = "2"
	red     = "31"
	green   = "32"
	yellow  = "33"
	blue    = "34"
	magenta = "35"
	cyan    = "36"
)

// style wraps a substring. A nil style leaves its group unpainted.
type style func(string) string

func sgr(codes ...string) style {
	prefix := "\x1b[" + strings.Join(codes, ";") + "m"
	return func(s string) string { return prefix + s + sgrReset }
}

// reANSI matches an SGR sequence — the only escape this package emits.
var reANSI = regexp.MustCompile(`\x1b\[[0-9;]*m`)

// StripANSI removes every escape sequence this package can produce. It exists so
// the tests can assert the round trip StripANSI(paint(s)) == s, which is the
// invariant the whole parity mandate rests on.
func StripANSI(s string) string { return reANSI.ReplaceAllString(s, "") }

// rule paints the capture groups of a line-anchored pattern. styles is aligned
// to the groups: styles[0] paints group 1, and a nil entry leaves that group
// alone. Groups must appear in order and must not overlap.
type rule struct {
	re     *regexp.Regexp
	styles []style
}

// rules are tried in order and the first match wins, so more specific patterns
// come first. Every pattern is anchored at the start of a line: gostow's output
// is a small, closed set of shapes, and matching them positionally keeps a
// filename from being mistaken for a keyword — except at the very start of a
// line, where a target path could in principle spell "LINK:". That would mistint
// one line on one terminal, and cannot alter a byte on a pipe.
var rules = []rule{
	// Engine operation log — one line per filesystem mutation. Additive
	// operations are green, destructive ones red, structural ones blue. RMDIR
	// prints without a colon: that is stow's, faithfully reproduced (ledger
	// PL-05), and the pattern must not quietly add one back.
	{regexp.MustCompile(`^(LINK:)`), []style{sgr(green)}},
	{regexp.MustCompile(`^(UNLINK:)`), []style{sgr(red)}},
	{regexp.MustCompile(`^(MKDIR:)`), []style{sgr(blue)}},
	{regexp.MustCompile(`^(RMDIR)(\s|$)`), []style{sgr(blue), nil}},
	{regexp.MustCompile(`^(MV:)`), []style{sgr(cyan)}},

	// Diagnostics. "<prog>: ERROR:" is Stow::Util::error()'s shape; the prog
	// prefix is left unpainted because it is an identity, not a severity.
	{regexp.MustCompile(`^(\S*: )(ERROR:)`), []style{nil, sgr(bold, red)}},
	{regexp.MustCompile(`^(WARNING!)`), []style{sgr(bold, yellow)}},
	{regexp.MustCompile(`^(WARNING:)`), []style{sgr(yellow)}},
	{regexp.MustCompile(`^(  )(\*)( )`), []style{nil, sgr(red), nil}},
	{regexp.MustCompile(`^(All operations aborted\.)$`), []style{sgr(red)}},

	// Verbose trace lines are prefixed "--- " by the engine. Dimming the prefix
	// separates the running commentary from the operations that matter.
	{regexp.MustCompile(`^(--- )`), []style{sgr(dim)}},

	// The help block. Colour tints it in place and may never re-lay it out: the
	// round-trip test asserts that stripping the escapes returns it unchanged.
	{regexp.MustCompile(`^(gostow \S+ \(GNU Stow \S+ compatible\))$`), []style{sgr(bold)}},
	// A section heading is a line of capitals ending in a colon. Anchoring to
	// the whole line keeps "Report gostow's bugs to: ..." out of it.
	{regexp.MustCompile(`^([A-Z][A-Z ]*:)$`), []style{sgr(bold)}},
	// gostow's own flags are magenta, so a reader can see at a glance which
	// lines are not GNU Stow's. This rule precedes the general option rule
	// because "--gostow-fix" also matches that one.
	{regexp.MustCompile(`^(    )(--gostow-\S*)(\s\s+)`), []style{nil, sgr(magenta), nil}},
	// An option line is four spaces, a flag, then at least two spaces of gutter.
	// Continuation lines are indented further and are left alone.
	{regexp.MustCompile(`^(    )(-\S.*?)(\s\s+)`), []style{nil, sgr(cyan), nil}},
}

// paint applies the first matching rule to a single line (which must not contain
// a newline), returning it unchanged when nothing matches.
func paint(line string) string {
	for _, r := range rules {
		m := r.re.FindStringSubmatchIndex(line)
		if m == nil {
			continue
		}
		var b strings.Builder
		prev := 0
		for g, st := range r.styles {
			lo, hi := m[2*(g+1)], m[2*(g+1)+1]
			if st == nil || lo < 0 {
				continue
			}
			b.WriteString(line[prev:lo])
			b.WriteString(st(line[lo:hi]))
			prev = hi
		}
		b.WriteString(line[prev:])
		return b.String()
	}
	return line
}

// Enabled reports whether w should be painted: a character device, with NO_COLOR
// unset and a terminal that can render escapes.
//
// A non-*os.File — the bytes.Buffer every test writes into — is never a TTY, so
// the test suite and the differential harness observe uncoloured output without
// having to ask for it.
func Enabled(w io.Writer) bool {
	// https://no-color.org: any non-empty value disables colour.
	if os.Getenv("NO_COLOR") != "" {
		return false
	}
	if os.Getenv("TERM") == "dumb" {
		return false
	}
	f, ok := w.(*os.File)
	if !ok {
		return false
	}
	info, err := f.Stat()
	if err != nil {
		return false
	}
	return info.Mode()&os.ModeCharDevice != 0
}

// Writer paints whole lines on their way to an underlying stream.
//
// When colour is disabled it is a pass-through: Write hands the slice straight
// to w, so the pipe sees exactly the bytes the caller produced and no code in
// this package can affect them. That is the parity guarantee, and it is why the
// disabled path is a single branch rather than a colourless rendering pass.
//
// When enabled, output is buffered to a line boundary, because a rule must see a
// complete line to match it. Callers must Flush a trailing partial line; every
// line gostow writes ends in "\n", so Flush is a safety net rather than a
// requirement.
type Writer struct {
	w       io.Writer
	enabled bool
	buf     []byte
}

// NewWriter wraps w. Pass Enabled(w) unless a test is forcing the decision.
func NewWriter(w io.Writer, enabled bool) *Writer {
	return &Writer{w: w, enabled: enabled}
}

func (p *Writer) Write(b []byte) (int, error) {
	if !p.enabled {
		return p.w.Write(b)
	}
	// Report every byte consumed: the caller handed us n bytes and we own them
	// now, whether they reached the stream or are held in buf awaiting a "\n".
	n := len(b)
	p.buf = append(p.buf, b...)
	for {
		i := indexNewline(p.buf)
		if i < 0 {
			return n, nil
		}
		line := string(p.buf[:i])
		p.buf = p.buf[i+1:]
		if _, err := io.WriteString(p.w, paint(line)+"\n"); err != nil {
			return n, err
		}
	}
}

// Flush emits any buffered partial line, painted.
func (p *Writer) Flush() error {
	if !p.enabled || len(p.buf) == 0 {
		return nil
	}
	line := string(p.buf)
	p.buf = nil
	_, err := io.WriteString(p.w, paint(line))
	return err
}

func indexNewline(b []byte) int {
	for i, c := range b {
		if c == '\n' {
			return i
		}
	}
	return -1
}
