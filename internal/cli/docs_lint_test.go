package cli

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

// The tests in docs_test.go ask whether the man page and the completion scripts
// say the right things. These ask whether they are well-formed at all: a man page
// groff refuses to set, or a completion script whose shell will not parse it, is
// worse than none — the second one breaks the user's shell startup.
//
// They need groff, bash, zsh and fish, and a missing tool must not turn into a
// silent pass. `go test ./...` on a laptop skips the tools it lacks and says so;
// CI sets GOSTOW_REQUIRE_DOC_TOOLS=1, which turns every skip into a failure. That
// is the same defence OraclePath makes in the conformance harness, for the same
// reason: with no oracle installed, the old differential suite printed `ok` in
// 0.26s and verified nothing.
const requireDocTools = "GOSTOW_REQUIRE_DOC_TOOLS"

func docTool(t *testing.T, name string) string {
	t.Helper()
	path, err := exec.LookPath(name)
	if err == nil {
		return path
	}
	if os.Getenv(requireDocTools) != "" {
		t.Fatalf("%s is not installed, and %s=1 says it must be", name, requireDocTools)
	}
	t.Skipf("%s is not installed; set %s=1 to make this a failure (CI does)", name, requireDocTools)
	return ""
}

func manPagePath(t *testing.T) string {
	t.Helper()
	dir, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	for {
		if _, err := os.Stat(filepath.Join(dir, "go.mod")); err == nil {
			return filepath.Join(dir, "man", "gostow.8")
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			t.Fatal("go.mod not found above the working directory")
		}
		dir = parent
	}
}

// groff warns about a byte above 0x7F wherever it appears — including inside a
// `.\"` comment, which is how this first bit the file: four em dashes in the
// header produced eight warnings and no visible defect. Checking the bytes needs
// no tools, so unlike the rest of this file it always runs.
func TestManPageIsPureASCII(t *testing.T) {
	src, err := os.ReadFile(manPagePath(t))
	if err != nil {
		t.Fatal(err)
	}
	for i, b := range src {
		if b > 0x7F {
			line := 1 + strings.Count(string(src[:i]), "\n")
			t.Fatalf("man/gostow.8:%d: byte %#x is not ASCII; groff warns on it even in a comment. "+
				"Use a roff escape (\\(em, \\(co, \\(bu) instead", line, b)
		}
	}
}

// -z sets the page and throws away the output, so anything on stderr is a real
// complaint about the source. -ww asks for all of them.
func TestManPageSetsWithoutWarnings(t *testing.T) {
	groff := docTool(t, "groff")
	path := manPagePath(t)

	cmd := exec.Command(groff, "-man", "-Tutf8", "-ww", "-z", path)
	var stderr strings.Builder
	cmd.Stderr = &stderr
	err := cmd.Run()

	if out := strings.TrimSpace(stderr.String()); out != "" {
		t.Errorf("groff complains about man/gostow.8:\n%s", out)
	}
	if err != nil {
		t.Errorf("groff exited non-zero: %v", err)
	}
}

// A completion script is sourced by the user's shell at startup. A syntax error
// in one is not a missing completion, it is a broken login.
func TestCompletionScriptsParse(t *testing.T) {
	root := filepath.Dir(filepath.Dir(manPagePath(t)))

	for _, c := range []struct {
		shell string
		file  string
		args  []string
	}{
		{"bash", "gostow.bash", []string{"-n"}},
		{"zsh", "_gostow", []string{"-n"}},
		{"fish", "gostow.fish", []string{"--no-execute"}},
	} {
		t.Run(c.shell, func(t *testing.T) {
			bin := docTool(t, c.shell)
			path := filepath.Join(root, "completions", c.file)

			out, err := exec.Command(bin, append(c.args, path)...).CombinedOutput()
			if err != nil {
				t.Errorf("%s cannot parse completions/%s: %v\n%s", c.shell, c.file, err, out)
			}
			if len(out) > 0 {
				t.Errorf("%s complains about completions/%s:\n%s", c.shell, c.file, out)
			}
		})
	}
}
