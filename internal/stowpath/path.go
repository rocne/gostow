// Package stowpath holds ports of GNU Stow's path helpers that both the engine
// and the CLI need.
//
// It exists because they were copied instead of shared, and the copy was wrong.
// `bin/stow` derives a default target with `parent($stow_dir) || '.'`, and
// internal/cli carried its own `parentOf` for that one call. On a single-segment
// absolute stow directory the two disagreed:
//
//	parent("/tmp")    == ""    → stow targets the cwd
//	parentOf("/tmp")  == "/"   → gostow targeted the filesystem root
//
// So `stow -d /tmp pkg` built its symlink farm in "/". The engine's copy was
// tested and correct; the CLI's was untested, because every fixture passed an
// explicit --target. One implementation, one set of tests, one behaviour.
package stowpath

import (
	"regexp"
	"strings"
)

var reSlashRun = regexp.MustCompile(`/+`)

// Parent returns everything above the last path segment, porting
// Stow::Util::parent.
//
// Runs of slashes count as one separator and a trailing slash is not a segment,
// so Parent("a/b/c/") is "a/b". A single-segment path has parent "" — including
// "/foo", because Perl's split leaves a leading empty field that is then dropped
// with the last segment. Callers that need a directory substitute "." for "",
// exactly as bin/stow's `|| '.'` does.
func Parent(paths ...string) string {
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
