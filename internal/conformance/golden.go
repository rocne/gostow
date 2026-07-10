package conformance

import (
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"testing"
)

// Golden is what real stow 2.4.1 did with a Case: the layer-5 recording that
// lets the everyday, Perl-free `go test ./...` check gostow against the oracle's
// answers without an oracle present.
//
// A golden is never hand-written. It is produced by the differential suite under
// `-update-goldens`, which refuses to run against anything but the pinned oracle,
// and it is re-derived from the live binary on every PR by the conformance job.
// Goldens alone would let a transcription error become permanent truth; a live
// oracle alone would put Perl on every contributor's machine. The pair is the
// point.
type Golden struct {
	ExitCode int
	Stdout   string
	Stderr   string
	Tree     string
}

const (
	goldenDir     = "testdata/goldens"
	goldenExit    = "### exit "
	goldenStdout  = "### stdout"
	goldenStderr  = "### stderr"
	goldenTree    = "### tree"
	goldenTrailer = "### end"
)

// Encode renders g as a reviewable text file. A diff of a golden should read as a
// diff of stow's behaviour, so the sections are plain and in a fixed order.
func Encode(g Golden) (string, error) {
	for name, section := range map[string]string{"stdout": g.Stdout, "stderr": g.Stderr, "tree": g.Tree} {
		if strings.Contains(section, "\n### ") || strings.HasPrefix(section, "### ") {
			return "", fmt.Errorf("%s contains a line beginning \"### \", which the golden format reserves", name)
		}
	}
	var b strings.Builder
	fmt.Fprintf(&b, "%s%d\n", goldenExit, g.ExitCode)
	for _, s := range []struct{ header, body string }{
		{goldenStdout, g.Stdout},
		{goldenStderr, g.Stderr},
		{goldenTree, g.Tree},
	} {
		b.WriteString(s.header + "\n")
		b.WriteString(s.body)
		if s.body != "" && !strings.HasSuffix(s.body, "\n") {
			b.WriteString("\n")
		}
	}
	b.WriteString(goldenTrailer + "\n")
	return b.String(), nil
}

// Decode is Encode's inverse. It is strict: a golden that does not round-trip is
// a golden nobody can trust.
func Decode(s string) (Golden, error) {
	var g Golden
	if !strings.HasSuffix(s, goldenTrailer+"\n") {
		return g, fmt.Errorf("golden is truncated: no %q trailer", goldenTrailer)
	}
	s = strings.TrimSuffix(s, goldenTrailer+"\n")

	exitLine, rest, ok := strings.Cut(s, "\n")
	if !ok || !strings.HasPrefix(exitLine, goldenExit) {
		return g, fmt.Errorf("golden does not begin with %q", goldenExit)
	}
	code, err := strconv.Atoi(strings.TrimPrefix(exitLine, goldenExit))
	if err != nil {
		return g, fmt.Errorf("golden exit code: %w", err)
	}
	g.ExitCode = code

	body, ok := strings.CutPrefix(rest, goldenStdout+"\n")
	if !ok {
		return g, fmt.Errorf("golden: expected %q", goldenStdout)
	}
	g.Stdout, body, ok = strings.Cut(body, goldenStderr+"\n")
	if !ok {
		return g, fmt.Errorf("golden: expected %q", goldenStderr)
	}
	g.Stderr, g.Tree, ok = strings.Cut(body, goldenTree+"\n")
	if !ok {
		return g, fmt.Errorf("golden: expected %q", goldenTree)
	}
	return g, nil
}

var reNonSlug = regexp.MustCompile(`[^a-z0-9]+`)

// GoldenName maps a Case name to a stable file name. Two cases whose names differ
// only in punctuation would collide, so the caller checks for that.
func GoldenName(caseName string) string {
	return strings.Trim(reNonSlug.ReplaceAllString(strings.ToLower(caseName), "-"), "-")
}

// GoldenPath is where the golden for a case lives, relative to the package dir.
func GoldenPath(caseName string) string {
	return filepath.Join(goldenDir, GoldenName(caseName)+".txt")
}

// LoadGolden reads the golden for c, failing loudly when it is absent. A missing
// golden must never be a skip: the whole layer would evaporate the first time
// somebody forgot to regenerate.
func LoadGolden(t *testing.T, caseName string) Golden {
	t.Helper()

	path := GoldenPath(caseName)
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("no golden for %q at %s: %v\n\nRegenerate with:\n\tPATH=$PWD/.oracle/bin:$PATH go test -count=1 -tags oracle ./internal/conformance/ -update-goldens",
			caseName, path, err)
	}
	g, err := Decode(string(data))
	if err != nil {
		t.Fatalf("golden %s is unreadable: %v", path, err)
	}
	return g
}

// SaveGolden writes the golden for a case, creating testdata/goldens as needed.
func SaveGolden(caseName string, g Golden) error {
	if err := os.MkdirAll(goldenDir, 0o755); err != nil {
		return err
	}
	encoded, err := Encode(g)
	if err != nil {
		return fmt.Errorf("%s: %w", caseName, err)
	}
	if round, err := Decode(encoded); err != nil || round != g {
		return fmt.Errorf("%s: golden does not round-trip through Encode/Decode", caseName)
	}
	return os.WriteFile(GoldenPath(caseName), []byte(encoded), 0o644)
}
