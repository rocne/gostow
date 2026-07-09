// Package stow is GNU Stow's symlink-farm engine, reimplemented in Go.
//
// The public surface is deliberately narrow — Apply, and the Stow/Unstow/Restow
// sugar over it — hiding tree folding, conflict detection, dot-prefix
// translation and ignore resolution. See docs/SPEC.md §3.
package stow

import (
	"regexp"
	"strings"
)

var (
	reRepeatedSlash = regexp.MustCompile(`/{2,}`)
	reDotSegment    = regexp.MustCompile(`(?:/\.)+(?:/|$)`)
	reLeadingDotDir = regexp.MustCompile(`^(?:\./)+`)
	reRootParents   = regexp.MustCompile(`^/(?:\.\./)+`)
	reSlashRun      = regexp.MustCompile(`/+`)
)

// canonpath is a port of Perl's File::Spec::Unix::canonpath, which join_paths
// leans on three separate times. It is not filepath.Clean: Clean resolves ".."
// against the preceding segment, canonpath leaves "a/../b" alone. join_paths
// strips ".." itself, afterwards, with different rules (see removeParentRefs).
func canonpath(path string) string {
	if path == "" {
		return ""
	}
	path = reRepeatedSlash.ReplaceAllString(path, "/") // xx////xx  -> xx/xx
	path = reDotSegment.ReplaceAllString(path, "/")    // xx/././xx -> xx/xx
	if path != "./" {
		path = reLeadingDotDir.ReplaceAllString(path, "") // ./xx -> xx
	}
	path = reRootParents.ReplaceAllString(path, "/") // /../../xx -> /xx
	if path == "/.." {
		path = "/"
	}
	if path != "/" {
		path = strings.TrimSuffix(path, "/") // xx/ -> xx
	}
	return path
}

// removeParentRefs collapses "foo/.." pairs, porting the substitution
//
//	1 while $result =~ s,(^|/)(?!\.\.)[^/]+/\.\.(/|$),$1,;
//
// The negative lookahead has no RE2 equivalent, so the scan is written out. Note
// it rejects any segment *starting* with "..", not merely the segment "..", and
// that a leading "/" may be consumed as the boundary — both are load-bearing.
func removeParentRefs(path string) string {
	for {
		next, ok := stripOneParentRef(path)
		if !ok {
			return path
		}
		path = next
	}
}

func stripOneParentRef(path string) (string, bool) {
	for pos := 0; pos < len(path); pos++ {
		// Perl tries the alternation left to right: "^" before "/".
		if pos == 0 {
			if end, ok := matchSegmentParent(path, 0); ok {
				return path[end:], true
			}
		}
		if path[pos] != '/' {
			continue
		}
		if end, ok := matchSegmentParent(path, pos+1); ok {
			return path[:pos] + "/" + path[end:], true
		}
	}
	return path, false
}

// matchSegmentParent reports whether `[^/]+/\.\.(/|$)` matches at segStart with
// the segment not beginning "..", returning the offset just past the match.
func matchSegmentParent(path string, segStart int) (int, bool) {
	rest := path[segStart:]
	if strings.HasPrefix(rest, "..") {
		return 0, false
	}
	segEnd := segStart + strings.IndexByte(rest, '/')
	if segEnd < segStart || segEnd == segStart {
		return 0, false // no "/" follows, or the segment is empty
	}
	if !strings.HasPrefix(path[segEnd:], "/..") {
		return 0, false
	}
	end := segEnd + len("/..")
	switch {
	case end == len(path):
		return end, true
	case path[end] == '/':
		return end + 1, true
	default:
		return 0, false
	}
}

// joinPaths concatenates path fragments the way Stow::Util::join_paths does: an
// absolute fragment discards everything to its left, empty fragments vanish,
// and "foo/.." pairs are cancelled. The result is relative iff no fragment was
// absolute — which is what makes stow's symlinks relative.
func joinPaths(paths ...string) string {
	result := ""
	for _, part := range paths {
		if part == "" {
			continue
		}
		part = canonpath(part)
		if strings.HasPrefix(part, "/") {
			result = part // absolute: ignore all previous parts
			continue
		}
		if result != "" && result != "/" {
			result += "/"
		}
		result += part
	}
	result = canonpath(result)
	result = removeParentRefs(result)
	return canonpath(result)
}

// parent returns everything above the last path segment. Runs of slashes count
// as one separator and a trailing slash is not a segment, so parent("a/b/c/")
// is "a/b". A single-segment path has parent "".
func parent(paths ...string) string {
	path := strings.Join(paths, "/")
	elts := reSlashRun.Split(path, -1)
	// Perl's split discards trailing empty fields.
	for len(elts) > 0 && elts[len(elts)-1] == "" {
		elts = elts[:len(elts)-1]
	}
	if len(elts) == 0 {
		return ""
	}
	return strings.Join(elts[:len(elts)-1], "/")
}
