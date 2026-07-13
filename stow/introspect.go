package stow

import (
	"os"
	"path/filepath"
	"strings"
)

// Owner reports whether the symlink at path is owned by stow — that is, whether
// its destination lies inside stow directory dir — and if so, which package it
// belongs to. owned is false (and pkg empty) when the link points anywhere else,
// including at an absolute destination, which stow never owns.
//
// The resolution is exactly the engine's own: the link's destination is read and
// joined against the link's parent per stow's join_paths rules, and both a
// contained plain stow dir and a .stow-marked directory elsewhere are honoured.
// It exists so a consumer need not re-derive those pinned semantics — a second
// implementation that drifted from this one is the class of bug a symlink manager
// can least afford.
func Owner(dir, path string) (pkg string, owned bool, err error) {
	abs, err := filepath.Abs(path)
	if err != nil {
		return "", false, fatalf("cannot resolve path %s (%s)", path, errnoText(err))
	}
	dest, err := os.Readlink(abs)
	if err != nil {
		return "", false, fatalf("Could not read link: %s (%s)", path, errnoText(err))
	}

	// Frame the engine on the link's own directory as the target. findStowedPath
	// then resolves the destination relative to that directory and compares it
	// against the stow dir expressed in the same frame — the link's basename is
	// used only through Parent(), which discards it, so the single-segment name is
	// immaterial. Symlinks in the link's parent are resolved consistently on both
	// sides (canon_path on the target, the OS on the readlink), so the frames agree.
	e, err := newEngine(Options{Dir: dir, Target: filepath.Dir(abs)})
	if err != nil {
		return "", false, err
	}
	_, _, pkg = e.findStowedPath(filepath.Base(abs), dest)
	return pkg, pkg != "", nil
}

// Expected computes the link set that stowing pkg would create into an empty
// target: a pure function of the package tree and the given options. The target's
// *contents* are never read — only opts.Target's location is consulted, and only
// because a stow link's destination is expressed relative to it.
//
// The result is keyed by target-relative link path and valued by the
// package-relative source path — the path within pkg that the link stands for,
// which is what Owner recovers from a deployed link, so the two compose directly
// for drift detection.
//
// Folding is resolved as if the target were empty: with opts.Fold on (stow's
// default) a package directory yields a single entry, the fold link, and its
// contents are not descended into; with folding off, each leaf yields its own
// entry and a directory contributes only tree shape, no link. Comparing the two
// is how a caller detects that a deployed tree was folded under a setting the
// current configuration no longer produces.
//
// Ignore resolution (--ignore plus the .stow-*-ignore files and the built-in
// defaults) and --dotfiles translation apply exactly as in a real stow; the
// package's ignore files live in the stow dir, not the target, so reading them
// does not consult the target. A package node that is itself an absolute symlink
// produces no entry, matching stow's refusal to represent one as a relative link.
func Expected(opts Options, pkg string) (map[string]string, error) {
	e, err := newEngine(opts)
	if err != nil {
		return nil, err
	}
	if !e.dirExists(e.packagePath(pkg)) {
		return nil, fatalf("The stow directory %s does not contain package %s", e.stowPath, pkg)
	}
	out := map[string]string{}
	if err := e.expectContents(e.stowPath, pkg, ".", ".", out); err != nil {
		return nil, err
	}
	return out, nil
}

// expectContents mirrors stowContents for the empty-target case: it walks the
// package directory, applying ignore and dot-prefix translation identically, but
// never inspects the target tree. See Expected.
func (e *engine) expectContents(stowPath, pkg, pkgSubdir, targetSubdir string, out map[string]string) error {
	pkgPath := joinPaths(stowPath, pkg, pkgSubdir)
	names, err := e.readdirSorted(pkgPath)
	if err != nil {
		return err
	}
	for _, node := range names {
		packageNodePath := joinPaths(pkgSubdir, node)
		targetNode := node
		targetNodePath := joinPaths(targetSubdir, targetNode)

		ignored, err := e.ignore(stowPath, pkg, targetNodePath)
		if err != nil {
			return err
		}
		if ignored {
			continue
		}
		if e.opts.Dotfiles {
			if adjusted := adjustDotfile(node); adjusted != node {
				targetNode = adjusted
				targetNodePath = joinPaths(targetSubdir, targetNode)
			}
		}
		if err := e.expectNode(stowPath, pkg, packageNodePath, targetNodePath, out); err != nil {
			return err
		}
	}
	return nil
}

// expectNode is stowNode reduced to the branches an empty target can reach: an
// absolute symlink in the package yields nothing (stow would raise a conflict); a
// real directory under --no-folding is descended into and contributes only shape;
// everything else — every leaf, and every directory when folding is on — is one
// link, recorded as target path => package-relative source.
func (e *engine) expectNode(stowPath, pkg, pkgSubpath, targetSubpath string, out map[string]string) error {
	pkgPath := joinPaths(stowPath, pkg, pkgSubpath)

	if fi, err := os.Lstat(e.real(pkgPath)); err == nil && fi.Mode()&os.ModeSymlink != 0 {
		dest, err := os.Readlink(e.real(pkgPath))
		if err != nil {
			return fatalf("Could not read link: %s (%s)", pkgPath, errnoText(err))
		}
		if strings.HasPrefix(dest, "/") {
			return nil
		}
	}

	if !e.opts.Fold && e.isRealDir(pkgPath) {
		return e.expectContents(stowPath, pkg, pkgSubpath, targetSubpath, out)
	}

	out[targetSubpath] = pkgSubpath
	return nil
}
