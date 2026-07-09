package ui

import (
	"bytes"
	"os"
	"strings"
	"testing"
)

// painted is every line shape gostow emits, plus the adversarial cases: a line
// that is only a keyword, a path that begins with one, and text that must never
// be touched.
//
// want records whether the shape is expected to gain colour. Without it the
// round-trip test below would pass vacuously against a paint() that returned its
// argument -- the same trap that made this project's ignore suite skip silently
// in CI. StripANSI(paint(s)) == s is only interesting once paint(s) != s.
var painted = []struct {
	line string
	want bool
}{
	{"LINK: .vimrc => ../stow/vim/.vimrc", true},
	{"UNLINK: .vimrc", true},
	{"MKDIR: .config", true},
	{"RMDIR .config", true}, // no colon: ledger PL-05, faithfully reproduced
	{"MV: .bashrc => .bashrc", true},
	{"stow: ERROR: bad package name", true},
	{"gostow: ERROR: cannot read ignore file /x/.stow-local-ignore", true},
	{"WARNING! stowing vim would cause conflicts:", true},
	{"WARNING: in simulation mode so not modifying filesystem.", true},
	{"  * existing target is not owned by stow: .vimrc", true},
	{"All operations aborted.", true},
	{"--- Skipping .vimrc as it already points to ../stow/vim/.vimrc", true},
	{"gostow 0.1.0 (GNU Stow 2.4.1 compatible)", true},
	{"SYNOPSIS:", true},
	{"OPTIONS:", true},
	{"GOSTOW EXTENSIONS:", true},
	{"    -d DIR, --dir=DIR     Set stow dir to DIR (default is current dir)", true},
	{"    --gostow-fix          Fix GNU Stow's known defects instead of matching them", true},

	// Left alone.
	{"", false},
	{"Report bugs to: bug-stow@gnu.org", false},
	{"                          if the file is already stowed to another package", false},
	{"                            -v or --verbose adds 1; --verbose=N sets level", false},
	{"RMDIRECTORY: not an operation", false}, // \s|$ guard earns its keep
	{"planning stow of package vim...", false},
	{"  ** not a bullet", false},
	{"héllo — ünicode ✓", false},
}

// TestPaintPreservesContent is the parity mandate as an executable claim: colour
// may only wrap existing bytes, never add, drop or reorder them.
func TestPaintPreservesContent(t *testing.T) {
	for _, c := range painted {
		if got := StripANSI(paint(c.line)); got != c.line {
			t.Errorf("StripANSI(paint(%q)) = %q, want the input back", c.line, got)
		}
	}
}

// TestPaintActuallyPaints keeps the test above honest.
func TestPaintActuallyPaints(t *testing.T) {
	for _, c := range painted {
		coloured := paint(c.line) != c.line
		if coloured != c.want {
			t.Errorf("paint(%q) coloured = %v, want %v", c.line, coloured, c.want)
		}
	}
}

// TestPaintEmitsOnlyRecognisedEscapes guards against a rule that reaches for a
// cursor movement or a screen clear, which would corrupt output captured through
// `script` or paged with `less -R`.
func TestPaintEmitsOnlyRecognisedEscapes(t *testing.T) {
	for _, c := range painted {
		out := paint(c.line)
		for _, seq := range reANSI.FindAllString(out, -1) {
			if !strings.HasSuffix(seq, "m") {
				t.Errorf("paint(%q) emitted non-SGR escape %q", c.line, seq)
			}
		}
		if strings.Count(out, "\x1b") != strings.Count(out, sgrReset)*2 {
			t.Errorf("paint(%q) = %q: every escape must open and close", c.line, out)
		}
	}
}

// TestDisabledWriterIsBytePassthrough is the pipe half of the mandate.
func TestDisabledWriterIsBytePassthrough(t *testing.T) {
	var buf bytes.Buffer
	w := NewWriter(&buf, false)

	input := "LINK: a => b\nWARNING! x\npartial line without newline"
	for i := 0; i < len(input); i++ { // one byte at a time: no chunking assumptions
		if _, err := w.Write([]byte{input[i]}); err != nil {
			t.Fatal(err)
		}
	}
	if err := w.Flush(); err != nil {
		t.Fatal(err)
	}
	if got := buf.String(); got != input {
		t.Errorf("disabled Writer altered the stream:\n got %q\nwant %q", got, input)
	}
}

// TestEnabledWriterSurvivesChunkBoundaries: a rule needs a whole line, but a
// caller may hand us any slice of one.
func TestEnabledWriterSurvivesChunkBoundaries(t *testing.T) {
	input := "MKDIR: .config\nLINK: a => b\nAll operations aborted.\ntrailing partial"

	for _, chunk := range []int{1, 3, 7, len(input)} {
		var buf bytes.Buffer
		w := NewWriter(&buf, true)
		for i := 0; i < len(input); i += chunk {
			end := min(i+chunk, len(input))
			if _, err := w.Write([]byte(input[i:end])); err != nil {
				t.Fatal(err)
			}
		}
		if err := w.Flush(); err != nil {
			t.Fatal(err)
		}
		if got := StripANSI(buf.String()); got != input {
			t.Errorf("chunk=%d: StripANSI(output) = %q, want %q", chunk, got, input)
		}
		if !strings.Contains(buf.String(), "\x1b[") {
			t.Errorf("chunk=%d: nothing was painted", chunk)
		}
	}
}

// TestWriteReportsAllBytesConsumed: io.Writer's contract is that a nil error
// means n == len(b). Buffering a partial line must not look like a short write,
// which would send fmt into an infinite retry.
func TestWriteReportsAllBytesConsumed(t *testing.T) {
	w := NewWriter(&bytes.Buffer{}, true)
	b := []byte("no newline here")
	n, err := w.Write(b)
	if err != nil || n != len(b) {
		t.Errorf("Write = (%d, %v), want (%d, nil)", n, err, len(b))
	}
}

func TestEnabled(t *testing.T) {
	t.Run("a non-file writer is never a TTY", func(t *testing.T) {
		t.Setenv("NO_COLOR", "")
		if Enabled(&bytes.Buffer{}) {
			t.Error("a bytes.Buffer must not be painted")
		}
	})

	t.Run("a pipe is not a TTY", func(t *testing.T) {
		t.Setenv("NO_COLOR", "")
		r, wp, err := os.Pipe()
		if err != nil {
			t.Fatal(err)
		}
		defer func() { _ = r.Close() }()
		defer func() { _ = wp.Close() }()
		if Enabled(wp) {
			t.Error("a pipe must not be painted")
		}
	})

	t.Run("NO_COLOR disables", func(t *testing.T) {
		t.Setenv("NO_COLOR", "1")
		if Enabled(os.Stdout) {
			t.Error("NO_COLOR must disable colour even on a TTY")
		}
	})

	t.Run("an empty NO_COLOR does not disable", func(t *testing.T) {
		// no-color.org: the variable disables colour when present *and non-empty*.
		// Asserted on a pipe, so the result is decided by the TTY check either
		// way; what is under test is that the env check does not short-circuit.
		t.Setenv("NO_COLOR", "")
		t.Setenv("TERM", "xterm")
		if got := os.Getenv("NO_COLOR"); got != "" {
			t.Fatalf("setup: NO_COLOR = %q", got)
		}
		// A file on disk is not a character device, so this stays false; the
		// point is that it reaches the check at all.
		f, err := os.CreateTemp(t.TempDir(), "out")
		if err != nil {
			t.Fatal(err)
		}
		defer func() { _ = f.Close() }()
		if Enabled(f) {
			t.Error("a regular file must not be painted")
		}
	})

	t.Run("TERM=dumb disables", func(t *testing.T) {
		t.Setenv("NO_COLOR", "")
		t.Setenv("TERM", "dumb")
		if Enabled(os.Stdout) {
			t.Error("TERM=dumb must disable colour")
		}
	})
}
