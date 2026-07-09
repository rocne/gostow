package stow

import (
	"fmt"
	"io"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
)

// Action is what a Request does with its packages.
type Action int

const (
	ActionStow Action = iota
	ActionUnstow
	ActionRestow
)

func (a Action) String() string {
	switch a {
	case ActionUnstow:
		return "unstow"
	case ActionRestow:
		return "restow"
	default:
		return "stow"
	}
}

// Request is one positional action group, mirroring stow's `-D pkg1 -S pkg2`.
type Request struct {
	Action   Action
	Packages []string
}

// Options configures an Apply. Dir and Target are required and must exist.
type Options struct {
	Dir       string
	Target    string
	Fold      bool // true is stow's default; the CLI's --no-folding sets false
	Dotfiles  bool
	Adopt     bool
	Compat    bool
	Simulate  bool
	Verbosity int
	Ignore    []string
	Defer     []string
	Override  []string
	Log       io.Writer // nil means io.Discard; the CLI passes os.Stderr
}

// Conflict is one reason an Apply refused to touch the filesystem.
type Conflict struct {
	Action  Action
	Package string
	Message string
}

// ConflictError is returned when planning found any conflict. Nothing was
// written: stow's semantics are all-or-nothing across the whole invocation.
type ConflictError struct{ Conflicts []Conflict }

func (e *ConflictError) Error() string {
	return fmt.Sprintf("%d conflict(s) detected", len(e.Conflicts))
}

// Result reports what an Apply planned and, unless Simulate, executed.
type Result struct {
	Tasks     []Task
	Conflicts []Conflict
}

type engine struct {
	opts       Options
	log        io.Writer
	targetPath string // absolute, symlinks resolved
	stowPath   string // the stow dir, relative to the target dir
	plan       *planner
	conflicts  []Conflict

	ignoreRE   []*regexp.Regexp
	deferRE    []*regexp.Regexp
	overrideRE []*regexp.Regexp

	ignoreLists map[string]*ignoreList // memoized per ignore-file path
}

// real maps a path expressed relative to the target directory — which is how
// stow names every path it prints — onto the filesystem. stow instead chdir()s
// into the target and uses the relative path directly; doing that here would
// mean mutating process-global state, so the join happens explicitly.
func (e *engine) real(p string) string {
	return filepath.Join(e.targetPath, filepath.FromSlash(p))
}

// Stow, Unstow and Restow are sugar over Apply.
func Stow(opts Options, pkgs ...string) (*Result, error) {
	return Apply(opts, Request{Action: ActionStow, Packages: pkgs})
}

func Unstow(opts Options, pkgs ...string) (*Result, error) {
	return Apply(opts, Request{Action: ActionUnstow, Packages: pkgs})
}

func Restow(opts Options, pkgs ...string) (*Result, error) {
	return Apply(opts, Request{Action: ActionRestow, Packages: pkgs})
}

// Apply plans every request, collects every conflict across all of them, and
// only then touches the filesystem. All unstow requests are planned before any
// stow request, whatever order they were given in — that is what makes
// `stow -D a -S b` and `stow -R a` behave as one atomic operation.
func Apply(opts Options, reqs ...Request) (*Result, error) {
	e, err := newEngine(opts)
	if err != nil {
		return nil, err
	}

	var toUnstow, toStow []string
	for _, r := range reqs {
		switch r.Action {
		case ActionUnstow:
			toUnstow = append(toUnstow, r.Packages...)
		case ActionStow:
			toStow = append(toStow, r.Packages...)
		case ActionRestow:
			toUnstow = append(toUnstow, r.Packages...)
			toStow = append(toStow, r.Packages...)
		}
	}

	if err := e.planUnstow(toUnstow); err != nil {
		return nil, err
	}
	if err := e.planStow(toStow); err != nil {
		return nil, err
	}

	if len(e.conflicts) > 0 {
		return &Result{Conflicts: e.conflicts}, &ConflictError{Conflicts: e.conflicts}
	}

	if !opts.Simulate {
		if err := e.processTasks(); err != nil {
			return nil, err
		}
	}

	res := &Result{}
	for _, t := range e.plan.tasks {
		if !t.skip {
			res.Tasks = append(res.Tasks, *t)
		}
	}
	return res, nil
}

func newEngine(opts Options) (*engine, error) {
	log := opts.Log
	if log == nil {
		log = io.Discard
	}
	e := &engine{opts: opts, log: log, plan: newPlanner(), ignoreLists: map[string]*ignoreList{}}

	target, err := canonPath(opts.Target)
	if err != nil {
		return nil, err
	}
	dir, err := canonPath(opts.Dir)
	if err != nil {
		return nil, err
	}
	e.targetPath = target

	rel, err := filepath.Rel(target, dir)
	if err != nil {
		return nil, fatalf("cannot express stow dir %s relative to target %s", dir, target)
	}
	e.stowPath = filepath.ToSlash(rel)

	e.debug(2, 0, "stow dir is %s", dir)
	e.debug(2, 0, "stow dir path relative to target %s is %s", target, e.stowPath)

	// --ignore is a suffix match, --defer and --override are prefix matches.
	// Perl's \z and \A are Go's $ and ^ so long as no (?m) flag is set.
	if e.ignoreRE, err = compileAll(opts.Ignore, "(%s)$"); err != nil {
		return nil, err
	}
	if e.deferRE, err = compileAll(opts.Defer, "^(%s)"); err != nil {
		return nil, err
	}
	if e.overrideRE, err = compileAll(opts.Override, "^(%s)"); err != nil {
		return nil, err
	}
	return e, nil
}

func compileAll(sources []string, anchor string) ([]*regexp.Regexp, error) {
	var out []*regexp.Regexp
	for _, s := range sources {
		re, err := regexp.Compile(fmt.Sprintf(anchor, s))
		if err != nil {
			return nil, fatalf("invalid regex %q: %v", s, err)
		}
		out = append(out, re)
	}
	return out, nil
}

// canonPath resolves symlinks and makes the path absolute, as Stow::Util's
// canon_path does. stow_path is derived from the canonical forms, so a symlinked
// target directory does not change the relative link destinations gostow writes.
func canonPath(p string) (string, error) {
	abs, err := filepath.Abs(p)
	if err != nil {
		return "", err
	}
	resolved, err := filepath.EvalSymlinks(abs)
	if err != nil {
		return abs, nil //nolint:nilerr // a not-yet-existing path canonicalises to its absolute form
	}
	return resolved, nil
}

func (e *engine) conflict(action Action, pkg, format string, args ...any) {
	e.conflicts = append(e.conflicts, Conflict{
		Action:  action,
		Package: pkg,
		Message: fmt.Sprintf(format, args...),
	})
}

func (e *engine) packagePath(pkg string) string {
	return joinPaths(e.stowPath, pkg)
}

func (e *engine) planUnstow(packages []string) error {
	if len(packages) == 0 {
		return nil
	}
	e.debug(2, 0, "Planning unstow of: %s ...", strings.Join(packages, " "))
	for _, pkg := range packages {
		if !e.dirExists(e.packagePath(pkg)) {
			return fatalf("The stow directory %s does not contain package %s", e.stowPath, pkg)
		}
		e.debug(2, 0, "Planning unstow of package %s...", pkg)
		if err := e.unstowContents(pkg, ".", "."); err != nil {
			return err
		}
		e.debug(2, 0, "Planning unstow of package %s... done", pkg)
	}
	return nil
}

func (e *engine) planStow(packages []string) error {
	if len(packages) == 0 {
		return nil
	}
	e.debug(2, 0, "Planning stow of: %s ...", strings.Join(packages, " "))
	for _, pkg := range packages {
		if !e.dirExists(e.packagePath(pkg)) {
			return fatalf("The stow directory %s does not contain package %s", e.stowPath, pkg)
		}
		e.debug(2, 0, "Planning stow of package %s...", pkg)
		if err := e.stowContents(e.stowPath, pkg, ".", "."); err != nil {
			return err
		}
		e.debug(2, 0, "Planning stow of package %s... done", pkg)
	}
	return nil
}

func (e *engine) dirExists(p string) bool {
	fi, err := os.Stat(e.real(p))
	return err == nil && fi.IsDir()
}

// readdirSorted lists a directory the way stow does: `sort readdir`, which is a
// bytewise sort, with "." and ".." dropped.
func (e *engine) readdirSorted(p string) ([]string, error) {
	entries, err := os.ReadDir(e.real(p))
	if err != nil {
		return nil, fatalf("cannot read directory: %s (%v)", p, err)
	}
	names := make([]string, 0, len(entries))
	for _, ent := range entries {
		names = append(names, ent.Name())
	}
	sort.Strings(names)
	return names, nil
}

// shouldSkipTarget protects stow directories from being stowed into.
//
// Ledger PL-04: stowContents passes the *package* subdir here while
// unstowContents passes the *target* subdir. Under --dotfiles those differ, so
// stowing bypasses a protection that unstowing honours. Replicated for v1.
func (e *engine) shouldSkipTarget(target string) bool {
	if target == e.stowPath {
		fmt.Fprintf(e.log, "WARNING: skipping target which was current stow directory %s\n", target)
		return true
	}
	if e.markedStowDir(target) {
		fmt.Fprintf(e.log, "WARNING: skipping marked Stow directory %s\n", target)
		return true
	}
	if e.exists(joinPaths(target, ".nonstow")) {
		fmt.Fprintf(e.log, "WARNING: skipping protected directory %s\n", target)
		return true
	}
	return false
}

func (e *engine) markedStowDir(dir string) bool {
	return e.exists(joinPaths(dir, ".stow"))
}

func (e *engine) exists(p string) bool {
	_, err := os.Stat(e.real(p))
	return err == nil
}

func (e *engine) stowContents(stowPath, pkg, pkgSubdir, targetSubdir string) error {
	if e.shouldSkipTarget(pkgSubdir) {
		return nil
	}
	e.debug(3, 0, "Stowing contents of %s / %s / %s", stowPath, pkg, pkgSubdir)
	e.debug(4, 1, "target subdir is %s", targetSubdir)

	pkgPath := joinPaths(stowPath, pkg, pkgSubdir)
	if !e.isANode(targetSubdir) {
		return fatalf("stow_contents() called with non-directory target: %s", targetSubdir)
	}
	names, err := e.readdirSorted(pkgPath)
	if err != nil {
		return err
	}

	for _, node := range names {
		packageNodePath := joinPaths(pkgSubdir, node)
		targetNode := node
		targetNodePath := joinPaths(targetSubdir, targetNode)

		// Ignore matching happens on the *untranslated* path, before the
		// dot-prefix adjustment below. See SPEC §6.
		ignored, err := e.ignore(stowPath, pkg, targetNodePath)
		if err != nil {
			return err
		}
		if ignored {
			continue
		}
		if e.opts.Dotfiles {
			if adjusted := adjustDotfile(node); adjusted != node {
				e.debug(4, 1, "Adjusting: %s => %s", node, adjusted)
				targetNode = adjusted
				targetNodePath = joinPaths(targetSubdir, targetNode)
			}
		}
		if err := e.stowNode(stowPath, pkg, packageNodePath, targetNodePath); err != nil {
			return err
		}
	}
	return nil
}

func (e *engine) stowNode(stowPath, pkg, pkgSubpath, targetSubpath string) error {
	e.debug(3, 0, "Stowing entry %s / %s / %s", stowPath, pkg, pkgSubpath)

	pkgPath := joinPaths(stowPath, pkg, pkgSubpath)

	// stow only ever writes relative links, so a package containing an absolute
	// symlink cannot be represented and is a conflict rather than a copy.
	if fi, err := os.Lstat(e.real(pkgPath)); err == nil && fi.Mode()&os.ModeSymlink != 0 {
		dest, err := os.Readlink(e.real(pkgPath))
		if err != nil {
			return fatalf("Could not read link: %s (%v)", pkgPath, err)
		}
		if strings.HasPrefix(dest, "/") {
			e.conflict(ActionStow, pkg, "source is an absolute symlink %s => %s", pkgPath, dest)
			return nil
		}
	}

	level := strings.Count(pkgSubpath, "/")
	e.debug(2, 1, "level of %s is %d", pkgSubpath, level)
	linkDest := joinPaths(strings.Repeat("../", level), pkgPath)
	e.debug(4, 1, "link destination %s", linkDest)

	switch {
	case e.isALink(targetSubpath):
		return e.stowOverExistingLink(stowPath, pkg, pkgSubpath, targetSubpath, linkDest)

	case e.isANode(targetSubpath):
		e.debug(4, 1, "Evaluate existing node: %s", targetSubpath)
		if e.isADir(targetSubpath) {
			if !e.dirExists(pkgPath) {
				e.conflict(ActionStow, pkg,
					"cannot stow non-directory %s over existing directory target %s", pkgPath, targetSubpath)
				return nil
			}
			return e.stowContents(e.stowPath, pkg, pkgSubpath, targetSubpath)
		}
		if !e.opts.Adopt {
			e.conflict(ActionStow, pkg,
				"cannot stow %s over existing target %s since neither a link nor a directory and --adopt not specified",
				pkgPath, targetSubpath)
			return nil
		}
		if e.dirExists(pkgPath) {
			e.conflict(ActionStow, pkg,
				"cannot stow directory %s over existing non-directory target %s", pkgPath, targetSubpath)
			return nil
		}
		e.doMv(targetSubpath, pkgPath)
		e.doLink(linkDest, targetSubpath)
		return nil

	case !e.opts.Fold && e.isRealDir(pkgPath):
		e.doMkdir(targetSubpath)
		return e.stowContents(e.stowPath, pkg, pkgSubpath, targetSubpath)

	default:
		e.doLink(linkDest, targetSubpath)
		return nil
	}
}

// isRealDir is -d && ! -l: a directory that is not reached through a symlink.
func (e *engine) isRealDir(p string) bool {
	fi, err := os.Lstat(e.real(p))
	return err == nil && fi.IsDir()
}

func (e *engine) stowOverExistingLink(stowPath, pkg, pkgSubpath, targetSubpath, linkDest string) error {
	existingLinkDest, err := e.readALink(targetSubpath)
	if err != nil {
		return err
	}
	e.debug(4, 1, "Evaluate existing link: %s => %s", targetSubpath, existingLinkDest)

	existingPkgPath, existingStowPath, existingPackage := e.findStowedPath(targetSubpath, existingLinkDest)
	if existingPkgPath == "" {
		e.conflict(ActionStow, pkg, "existing target is not owned by stow: %s", targetSubpath)
		return nil
	}

	if !e.isANode(existingPkgPath) {
		// The link points into a stow package at a path that no longer exists.
		e.debug(2, 0, "--- replacing invalid link: %s", targetSubpath)
		if err := e.doUnlink(targetSubpath); err != nil {
			return err
		}
		e.doLink(linkDest, targetSubpath)
		return nil
	}

	switch {
	case existingLinkDest == linkDest:
		e.debug(2, 0, "--- Skipping %s as it already points to %s", targetSubpath, linkDest)

	case e.matchAny(e.deferRE, targetSubpath):
		e.debug(2, 0, "--- Deferring installation of: %s", targetSubpath)

	case e.matchAny(e.overrideRE, targetSubpath):
		e.debug(2, 0, "--- Overriding installation of: %s", targetSubpath)
		if err := e.doUnlink(targetSubpath); err != nil {
			return err
		}
		e.doLink(linkDest, targetSubpath)

	case e.isADir(joinPaths(parent(targetSubpath), existingLinkDest)) &&
		e.isADir(joinPaths(parent(targetSubpath), linkDest)):
		// Both the folded tree and the newcomer are directories, so the fold is
		// split open and both packages are stowed into the real directory.
		e.debug(2, 0, "--- Unfolding %s which was already owned by %s", targetSubpath, existingPackage)
		if err := e.doUnlink(targetSubpath); err != nil {
			return err
		}
		e.doMkdir(targetSubpath)
		if err := e.stowContents(existingStowPath, existingPackage, pkgSubpath, targetSubpath); err != nil {
			return err
		}
		return e.stowContents(e.stowPath, pkg, pkgSubpath, targetSubpath)

	default:
		e.conflict(ActionStow, pkg,
			"existing target is stowed to a different package: %s => %s", targetSubpath, existingLinkDest)
	}
	return nil
}

func (e *engine) matchAny(res []*regexp.Regexp, path string) bool {
	for _, re := range res {
		if re.MatchString(path) {
			return true
		}
	}
	return false
}

func (e *engine) unstowContents(pkg, pkgSubdir, targetSubdir string) error {
	// Note the asymmetry with stowContents, which passes pkgSubdir. Ledger PL-04.
	if e.shouldSkipTarget(targetSubdir) {
		return nil
	}
	e.debug(3, 0, "Unstowing contents of %s / %s / %s", e.stowPath, pkg, pkgSubdir)
	e.debug(4, 1, "target subdir is %s", targetSubdir)

	pkgPath := joinPaths(e.stowPath, pkg, pkgSubdir)
	dir := pkgPath
	if e.opts.Compat {
		dir = targetSubdir
		if !e.dirExists(targetSubdir) {
			return fatalf("unstow_contents() in compat mode called with non-directory target: %s", targetSubdir)
		}
	} else {
		if !e.dirExists(pkgPath) {
			return fatalf("unstow_contents() called with non-directory path: %s", pkgPath)
		}
		if !e.isANode(targetSubdir) {
			return fatalf("unstow_contents() called with invalid target: %s", targetSubdir)
		}
	}

	names, err := e.readdirSorted(dir)
	if err != nil {
		return err
	}

	for _, node := range names {
		packageNode := node
		targetNode := node
		targetNodePath := joinPaths(targetSubdir, targetNode)

		ignored, err := e.ignore(e.stowPath, pkg, targetNodePath)
		if err != nil {
			return err
		}
		if ignored {
			continue
		}
		if e.opts.Dotfiles {
			if e.opts.Compat {
				// compat walks the target tree, so the translation runs backwards.
				if adjusted := unadjustDotfile(node); adjusted != node {
					e.debug(4, 1, "Reverse adjusting: %s => %s", node, adjusted)
					packageNode = adjusted
				}
			} else if adjusted := adjustDotfile(node); adjusted != node {
				e.debug(4, 1, "Adjusting: %s => %s", node, adjusted)
				targetNode = adjusted
				targetNodePath = joinPaths(targetSubdir, targetNode)
			}
		}
		if err := e.unstowNode(pkg, joinPaths(pkgSubdir, packageNode), targetNodePath); err != nil {
			return err
		}
	}

	if !e.opts.Compat && e.dirExists(targetSubdir) {
		return e.cleanupInvalidLinks(targetSubdir)
	}
	return nil
}

func (e *engine) unstowNode(pkg, pkgSubpath, targetSubpath string) error {
	e.debug(3, 0, "Unstowing entry from target: %s", targetSubpath)
	e.debug(4, 1, "Package entry: %s / %s / %s", e.stowPath, pkg, pkgSubpath)

	switch {
	case e.isALink(targetSubpath):
		return e.unstowLinkNode(pkg, pkgSubpath, targetSubpath)

	case e.dirExists(targetSubpath):
		if err := e.unstowContents(pkg, pkgSubpath, targetSubpath); err != nil {
			return err
		}
		parentInPkg, err := e.foldable(targetSubpath)
		if err != nil {
			return err
		}
		if parentInPkg != "" {
			return e.foldTree(targetSubpath, parentInPkg)
		}

	case e.exists(targetSubpath):
		e.debug(2, 1, "%s doesn't need to be unstowed", targetSubpath)

	default:
		e.debug(2, 1, "%s did not exist to be unstowed", targetSubpath)
	}
	return nil
}

func (e *engine) unstowLinkNode(pkg, pkgSubpath, targetSubpath string) error {
	e.debug(4, 2, "Evaluate existing link: %s", targetSubpath)

	linkDest, err := e.readALink(targetSubpath)
	if err != nil {
		return err
	}
	if strings.HasPrefix(linkDest, "/") {
		fmt.Fprintf(e.log, "Ignoring an absolute symlink: %s => %s\n", targetSubpath, linkDest)
		return nil
	}

	existingPkgPath, _, _ := e.findStowedPath(targetSubpath, linkDest)
	if existingPkgPath == "" {
		e.debug(5, 3, "Ignoring unowned link %s => %s", targetSubpath, linkDest)
		return nil
	}

	pkgPath := joinPaths(e.stowPath, pkg, pkgSubpath)
	if e.exists(existingPkgPath) {
		if existingPkgPath == pkgPath {
			return e.doUnlink(targetSubpath)
		}
		e.debug(5, 3, "Ignoring link %s => %s", targetSubpath, linkDest)
		return nil
	}
	e.debug(2, 0, "--- removing invalid link into a stow directory: %s", pkgPath)
	return e.doUnlink(targetSubpath)
}

// findStowedPath decides whether a link destination points into a stow package,
// and if so which. It returns ("", "", "") when the link is not owned by stow.
func (e *engine) findStowedPath(targetSubpath, linkDest string) (pkgPath, stowPath, pkg string) {
	if strings.HasPrefix(linkDest, "/") {
		return "", "", ""
	}
	candidate := joinPaths(parent(targetSubpath), linkDest)

	if p, _ := e.linkDestWithinStowDir(candidate); p != "" {
		return candidate, e.stowPath, p
	}
	if sp, ep := e.findContainingMarkedStowDir(candidate); sp != "" {
		return candidate, sp, ep
	}
	return "", "", ""
}

func (e *engine) linkDestWithinStowDir(linkDest string) (pkg, pkgSubpath string) {
	prefix := e.stowPath + "/"
	if !strings.HasPrefix(linkDest, prefix) {
		return "", ""
	}
	parts := strings.Split(strings.TrimPrefix(linkDest, prefix), "/")
	return parts[0], strings.Join(parts[1:], "/")
}

// findContainingMarkedStowDir supports the "multiple stow directories" feature:
// a link may point into a directory elsewhere that is marked with a .stow file.
func (e *engine) findContainingMarkedStowDir(pkgPath string) (stowPath, pkg string) {
	segments := strings.Split(pkgPath, "/")
	for last := range segments {
		candidate := joinPaths(segments[:last+1]...)
		if e.markedStowDir(candidate) {
			if last == len(segments)-1 {
				return "", "" // find_stowed_path() called directly on a stow dir
			}
			return candidate, segments[last+1]
		}
	}
	return "", ""
}

// cleanupInvalidLinks removes links that are both orphaned and owned by stow.
// They would otherwise block a refold, and they appear routinely when a file is
// renamed inside a package.
func (e *engine) cleanupInvalidLinks(dir string) error {
	e.debug(2, 0, "Cleaning up any invalid links in %s (pwd=%s)", dir, e.targetPath)

	names, err := e.readdirSorted(dir)
	if err != nil {
		return err
	}
	for _, node := range names {
		nodePath := joinPaths(dir, node)
		fi, err := os.Lstat(e.real(nodePath))
		if err != nil || fi.Mode()&os.ModeSymlink == 0 {
			continue
		}
		if t, ok := e.plan.linkTaskFor[nodePath]; ok {
			if t.Action != TaskRemove {
				fmt.Fprintf(e.log, "Unexpected action scheduled for %s; skipping clean-up\n", nodePath)
			}
			continue
		}
		linkDest, err := os.Readlink(e.real(nodePath))
		if err != nil {
			return fatalf("Could not read link %s", nodePath)
		}
		if e.exists(joinPaths(dir, linkDest)) {
			continue
		}
		if owner := e.linkOwnedByPackage(nodePath, linkDest); owner != "" {
			e.debug(2, 0, "--- removing link owned by %s: %s => %s", owner, nodePath, joinPaths(dir, linkDest))
			if err := e.doUnlink(nodePath); err != nil {
				return err
			}
		}
	}
	return nil
}

func (e *engine) linkOwnedByPackage(targetSubpath, linkDest string) string {
	_, _, pkg := e.findStowedPath(targetSubpath, linkDest)
	return pkg
}

// foldable reports the package-side parent directory that targetSubdir could
// collapse into, or "" if it cannot. A directory folds only when every node in
// it is a link, all the links share one parent inside the package, and that
// parent is itself owned by stow.
func (e *engine) foldable(targetSubdir string) (string, error) {
	e.debug(3, 2, "Is %s foldable?", targetSubdir)
	if !e.opts.Fold {
		return "", nil
	}

	names, err := e.readdirSorted(targetSubdir)
	if err != nil {
		return "", err
	}
	parentInPkg := ""
	for _, node := range names {
		targetNodePath := joinPaths(targetSubdir, node)
		if !e.isANode(targetNodePath) {
			continue
		}
		if !e.isALink(targetNodePath) {
			return "", nil
		}
		linkDest, err := e.readALink(targetNodePath)
		if err != nil {
			return "", err
		}
		newParent := parent(linkDest)
		if parentInPkg == "" {
			parentInPkg = newParent
		} else if parentInPkg != newParent {
			return "", nil
		}
	}
	if parentInPkg == "" {
		return "", nil // no links: nothing to fold into
	}
	parentInPkg = strings.TrimPrefix(parentInPkg, "../")
	if e.linkOwnedByPackage(targetSubdir, parentInPkg) != "" {
		return parentInPkg, nil
	}
	return "", nil
}

// foldTree replaces a directory of links with a single link to their shared
// package-side parent: UNLINK each node, RMDIR, then LINK.
func (e *engine) foldTree(targetSubdir, pkgSubpath string) error {
	e.debug(3, 0, "--- Folding tree: %s => %s", targetSubdir, pkgSubpath)
	names, err := e.readdirSorted(targetSubdir)
	if err != nil {
		return err
	}
	for _, node := range names {
		nodePath := joinPaths(targetSubdir, node)
		if !e.isANode(nodePath) {
			continue
		}
		if err := e.doUnlink(nodePath); err != nil {
			return err
		}
	}
	e.doRmdir(targetSubdir)
	e.doLink(pkgSubpath, targetSubdir)
	return nil
}

// FatalError is stow's error(): the CLI renders it as "<prog>: ERROR: <msg>" and
// exits 2. Every fatal exit is pinned to 2 because stow's own status is
// errno-derived and therefore undefined — ledger PL-07.
//
// It also keeps stow's message wording, which is capitalised and punctuated
// unlike a Go error string, out of fmt.Errorf's way: these strings are a
// byte-parity contract, not prose we are free to restyle.
type FatalError struct{ Msg string }

func (e *FatalError) Error() string { return e.Msg }

func fatalf(format string, args ...any) error {
	return &FatalError{Msg: fmt.Sprintf(format, args...)}
}
