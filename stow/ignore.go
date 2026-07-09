package stow

import (
	"bufio"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
)

const (
	localIgnoreFile  = ".stow-local-ignore"
	globalIgnoreFile = ".stow-global-ignore"
)

// defaultIgnoreData is Stow.pm's __DATA__ section verbatim. It is parsed by the
// very same reader as a user's ignore file, which is why it may carry comments
// and why the self-ignoring `^/\.stow-local-ignore$` rule ends up in it too.
const defaultIgnoreData = `
# Comments and blank lines are allowed.

RCS
.+,v

CVS
\.\#.+       # CVS conflict files / emacs lock files
\.cvsignore

\.svn
_darcs
\.hg

\.git
\.gitignore
\.gitmodules

.+~          # emacs backup files
\#.*\#       # emacs autosave files

^/README.*
^/LICENSE.*
^/COPYING
`

// ignoreList is one compiled ignore source. Either regexp may be nil, meaning
// that source contributed no patterns of that kind.
type ignoreList struct {
	path    *regexp.Regexp
	segment *regexp.Regexp
}

// ignore reports whether a target node should be skipped.
//
// --ignore patterns are additive and are checked before any file-based source.
// The three file-based sources are *exclusive*: the first that exists wins
// outright, so a ~/.stow-global-ignore discards the built-in defaults entirely.
func (e *engine) ignore(stowPath, pkg, target string) (bool, error) {
	if target == "" {
		return false, fatalf("Stow::ignore() called with empty target")
	}
	for _, re := range e.ignoreRE {
		if re.MatchString(target) {
			e.debug(4, 1, "Ignoring path %s due to --ignore", target)
			return true, nil
		}
	}

	list, err := e.ignoreRegexps(joinPaths(stowPath, pkg))
	if err != nil {
		return false, err
	}
	if list.path != nil && list.path.MatchString("/"+target) {
		e.debug(4, 1, "Ignoring path /%s", target)
		return true, nil
	}
	basename := target
	if i := strings.LastIndexByte(target, '/'); i >= 0 {
		basename = target[i+1:]
	}
	if list.segment != nil && list.segment.MatchString(basename) {
		e.debug(4, 1, "Ignoring path segment %s", basename)
		return true, nil
	}
	return false, nil
}

// ignoreRegexps resolves the three exclusive sources for one package directory,
// memoizing per file path for the lifetime of the engine as stow does.
func (e *engine) ignoreRegexps(packageDir string) (*ignoreList, error) {
	local := joinPaths(packageDir, localIgnoreFile)
	global := joinPaths(os.Getenv("HOME"), globalIgnoreFile)

	for _, file := range []string{local, global} {
		if !e.existsPath(file) {
			continue
		}
		if list, ok := e.ignoreLists[file]; ok {
			return list, nil
		}
		list, err := loadIgnoreFile(e.resolve(file))
		if err != nil {
			return nil, err
		}
		e.ignoreLists[file] = list
		return list, nil
	}

	const builtin = "\x00builtin"
	if list, ok := e.ignoreLists[builtin]; ok {
		return list, nil
	}
	// defaultIgnoreData is a compiled-in string, so the reader cannot fail.
	patterns, err := parseIgnoreReader(strings.NewReader(defaultIgnoreData))
	if err != nil {
		return nil, err
	}
	list, err := compileIgnorePatterns(patterns)
	if err != nil {
		return nil, err
	}
	e.ignoreLists[builtin] = list
	return list, nil
}

// resolve turns an ignore-file path into a filesystem path. A path derived from
// $HOME is already absolute; one derived from the stow dir is relative to the
// target directory.
func (e *engine) resolve(p string) string {
	if filepath.IsAbs(p) {
		return p
	}
	return e.real(p)
}

func (e *engine) existsPath(p string) bool {
	_, err := os.Stat(e.resolve(p))
	return err == nil
}

// loadIgnoreFile reads and compiles one ignore file.
//
// Ledger PL-10: real stow returns undef when the file exists but cannot be
// opened, which disables *all* ignoring — the built-in defaults included, so
// that the unreadable .stow-local-ignore proceeds to stow itself, silently and
// with exit 0. That is ruled a bug and is not replicated: an unreadable ignore
// file is a broken configuration and fails loudly.
func loadIgnoreFile(path string) (*ignoreList, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, fatalf("cannot read ignore file %s (%v)", path, err)
	}
	defer func() { _ = f.Close() }()

	patterns, err := parseIgnoreReader(f)
	if err != nil {
		return nil, fatalf("cannot read ignore file %s (%v)", path, err)
	}
	return compileIgnorePatterns(patterns)
}

// parseIgnoreReader applies stow's per-line cleanup and returns the distinct
// patterns. The self-ignoring rule is always appended, so a local ignore file
// never stows itself.
//
// A read error is returned, never swallowed. Silently treating a half-read file
// as a complete one would reproduce, by a different route, the very bug PL-10
// rules against: the three ignore sources are exclusive, so an ignore file that
// yields no patterns does not fall back to the built-in defaults — it disables
// them. `os.Open` succeeds on a *directory*, and its first Read fails with
// EISDIR; before this returned an error, a directory named `.stow-local-ignore`
// made gostow stow README.md and exit 0.
func parseIgnoreReader(r interface{ Read([]byte) (int, error) }) ([]string, error) {
	seen := map[string]bool{}
	var out []string
	add := func(p string) {
		if !seen[p] {
			seen[p] = true
			out = append(out, p)
		}
	}

	commentSuffix := regexp.MustCompile(`\s+#.+`)
	sc := bufio.NewScanner(r)
	for sc.Scan() {
		line := strings.TrimSpace(sc.Text())
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		line = commentSuffix.ReplaceAllString(line, "")
		line = strings.ReplaceAll(line, `\#`, "#")
		add(line)
	}
	if err := sc.Err(); err != nil {
		return nil, err
	}
	add(`^/\.stow-local-ignore$`)
	return out, nil
}

// compileIgnorePatterns partitions patterns by whether they contain a "/".
// Those without one match a node's basename; those with one match the whole
// target path, with a leading "/" prepended.
//
// Ledger PL-15: Perl's "$" also matches before a trailing newline, Go's does
// not. A filename may legally end in "\n", so stow would ignore "README\n"
// where gostow does not. RE2's stricter anchor is the safer reading — it never
// ignores a file the pattern did not name — and matching Perl here would mean
// hand-rolling "(\n?$)" into every user pattern. Ruled: keep RE2 semantics.
func compileIgnorePatterns(patterns []string) (*ignoreList, error) {
	var segments, paths []string
	for _, p := range patterns {
		if strings.Contains(p, "/") {
			paths = append(paths, p)
		} else {
			segments = append(segments, p)
		}
	}
	// stow joins these in Perl hash order, which is randomised per process. The
	// order cannot affect whether the alternation matches, only the regexp's
	// printed form at verbosity 5, so a stable order is chosen here.
	sort.Strings(segments)
	sort.Strings(paths)

	list := &ignoreList{}
	var err error
	if len(segments) > 0 {
		if list.segment, err = regexp.Compile("^(" + strings.Join(segments, "|") + ")$"); err != nil {
			return nil, fatalf("Failed to compile regexp: %v", err)
		}
	}
	if len(paths) > 0 {
		if list.path, err = regexp.Compile("(^|/)(" + strings.Join(paths, "|") + ")(/|$)"); err != nil {
			return nil, fatalf("Failed to compile regexp: %v", err)
		}
	}
	return list, nil
}
