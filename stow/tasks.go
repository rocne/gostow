package stow

import (
	"fmt"
	"os"
	"strings"
)

// TaskAction is what a planned task does to the filesystem.
type TaskAction int

const (
	TaskCreate TaskAction = iota
	TaskRemove
	TaskMove
)

// TaskType is what a planned task acts upon.
type TaskType int

const (
	TypeLink TaskType = iota
	TypeDir
	TypeFile
)

// Task is one planned filesystem mutation. Paths are relative to the target
// directory, matching the paths stow prints.
type Task struct {
	Action TaskAction
	Type   TaskType
	Path   string

	// Source is what a symlink points at — the destination recorded in the link
	// itself. Links only; empty otherwise. (stow's own name for it: a link's
	// "source" is its target, which reads backwards but is the vocabulary the
	// engine and its documentation share.)
	Source string

	// Dest is where a file moves to, under --adopt. Moves only; empty otherwise.
	//
	// Source and Dest are separate because they are separate in Stow.pm, whose
	// task comment reads "source => (only for links); dest => (only for moving
	// files)". Folding them into one field made a move's *destination* live in a
	// field called Source, which is not a shortcut but an error.
	Dest string

	// skip marks a task cancelled by a later, opposing task. stow leaves the
	// cancelled task in the list and filters it just before execution, which is
	// why cancellation cannot simply delete from the slice: task order for the
	// surviving tasks must not change.
	skip bool
}

// An operation is one line of stow's level-1 log. There is exactly one printer
// for all of them (logOp), because stow has none — it prints these ad hoc, and
// the inconsistency below is the visible consequence.
type operation struct {
	name string
	// colon is false for RMDIR alone. Every sibling prints "LINK:", "UNLINK:",
	// "MKDIR:", "MV:"; RMDIR prints "RMDIR /path". That is reproducible and
	// dependable, so parity replicates it verbatim — it is declared here rather
	// than hidden in a format string, where it reads as a typo somebody will
	// helpfully "fix". Options.FixQuirks gives it the colon its siblings have.
	colon bool
}

var (
	opLink   = operation{name: "LINK", colon: true}
	opUnlink = operation{name: "UNLINK", colon: true}
	opMkdir  = operation{name: "MKDIR", colon: true}
	opRmdir  = operation{name: "RMDIR", colon: false}
	opMv     = operation{name: "MV", colon: true}
)

// opNote is the parenthetical stow appends when a task cancels another.
type opNote string

const (
	noteNone      opNote = ""
	noteDuplicate opNote = " (duplicates previous action)"
	noteRevert    opNote = " (reverts previous action)"
)

// logOp writes one operation line. Every LINK/UNLINK/MKDIR/RMDIR/MV in the
// engine goes through here.
func (e *engine) logOp(op operation, note opNote, detail string) {
	separator := ":"
	if !op.colon && !e.opts.FixQuirks {
		separator = ""
	}
	e.debug(1, 0, "%s%s %s%s", op.name, separator, detail, note)
}

// planner holds the two-phase plan: an ordered task list plus the indices stow
// keeps beside it, which together form a virtual filesystem overlaid on the real
// one. Every predicate below asks the plan first and the disk second, so that
// planning can reason about a tree it has not created yet.
type planner struct {
	tasks       []*Task
	dirTaskFor  map[string]*Task
	linkTaskFor map[string]*Task
}

func newPlanner() *planner {
	return &planner{
		dirTaskFor:  map[string]*Task{},
		linkTaskFor: map[string]*Task{},
	}
}

func (p *planner) linkTaskAction(path string) (TaskAction, bool) {
	t, ok := p.linkTaskFor[path]
	if !ok {
		return 0, false
	}
	return t.Action, true
}

func (p *planner) dirTaskAction(path string) (TaskAction, bool) {
	t, ok := p.dirTaskFor[path]
	if !ok {
		return 0, false
	}
	return t.Action, true
}

// parentLinkScheduledForRemoval reports whether any ancestor of path — or path
// itself — is a link the plan intends to remove. If so, everything beneath it is
// already conceptually gone, even though the disk still shows it.
func (p *planner) parentLinkScheduledForRemoval(targetPath string) bool {
	prefix := ""
	for _, part := range reSlashRun.Split(targetPath, -1) {
		if part == "" {
			continue
		}
		prefix = joinPaths(prefix, part)
		if t, ok := p.linkTaskFor[prefix]; ok && t.Action == TaskRemove {
			return true
		}
	}
	return false
}

// isALink reports whether targetPath is, or is planned to become, a symlink.
func (e *engine) isALink(targetPath string) bool {
	if action, ok := e.plan.linkTaskAction(targetPath); ok {
		return action == TaskCreate
	}
	if fi, err := os.Lstat(e.real(targetPath)); err == nil && fi.Mode()&os.ModeSymlink != 0 {
		return !e.plan.parentLinkScheduledForRemoval(targetPath)
	}
	return false
}

// isADir reports whether targetPath is, or is planned to become, a directory.
// It follows symlinks, as Perl's -d does.
func (e *engine) isADir(targetPath string) bool {
	if action, ok := e.plan.dirTaskAction(targetPath); ok {
		return action == TaskCreate
	}
	if e.plan.parentLinkScheduledForRemoval(targetPath) {
		return false
	}
	fi, err := os.Stat(e.real(targetPath))
	return err == nil && fi.IsDir()
}

// isANode reports whether targetPath exists, or is planned to exist, as either a
// link or a directory. The combinations below are stow's own truth table; two of
// them are internal errors upstream, and are simply impossible states here.
func (e *engine) isANode(targetPath string) bool {
	laction, lok := e.plan.linkTaskAction(targetPath)
	daction, dok := e.plan.dirTaskAction(targetPath)

	switch {
	case lok && laction == TaskRemove:
		if dok && daction == TaskCreate {
			return true
		}
		return false
	case lok && laction == TaskCreate:
		return true
	default:
		if dok {
			return daction == TaskCreate
		}
	}

	if e.plan.parentLinkScheduledForRemoval(targetPath) {
		return false
	}
	_, err := os.Stat(e.real(targetPath))
	return err == nil
}

// readALink returns the destination of a current or planned link.
func (e *engine) readALink(link string) (string, error) {
	if t, ok := e.plan.linkTaskFor[link]; ok {
		if t.Action == TaskCreate {
			return t.Source, nil
		}
		return "", fatalf("read_a_link() passed a path that is scheduled for removal: %s", link)
	}
	dest, err := os.Readlink(e.real(link))
	if err != nil {
		return "", fatalf("Could not read link: %s (%v)", link, err)
	}
	// stow tests the destination for Perl truthiness here, so a link pointing at
	// the literal string "0" is reported unreadable. Ledger PL-09: ruled a bug,
	// not replicated. An empty destination is not creatable on Linux anyway.
	return dest, nil
}

func linkDetail(linkSrc, linkDest string) string {
	return fmt.Sprintf("%s => %s", linkSrc, linkDest)
}

func (e *engine) doLink(linkDest, linkSrc string) {
	if t, ok := e.plan.linkTaskFor[linkSrc]; ok {
		switch {
		case t.Action == TaskCreate && t.Source == linkDest:
			e.logOp(opLink, noteDuplicate, linkDetail(linkSrc, linkDest))
			return
		case t.Action == TaskRemove && t.Source == linkDest:
			e.logOp(opLink, noteRevert, linkDetail(linkSrc, linkDest))
			t.skip = true
			delete(e.plan.linkTaskFor, linkSrc)
			return
		}
		// A removal of a *different* destination falls through: the plan keeps
		// both the removal and this creation, which is how --override relinks.
	}

	e.logOp(opLink, noteNone, linkDetail(linkSrc, linkDest))
	t := &Task{Action: TaskCreate, Type: TypeLink, Path: linkSrc, Source: linkDest}
	e.plan.tasks = append(e.plan.tasks, t)
	e.plan.linkTaskFor[linkSrc] = t
}

func (e *engine) doUnlink(file string) error {
	if t, ok := e.plan.linkTaskFor[file]; ok {
		switch t.Action {
		case TaskRemove:
			e.logOp(opUnlink, noteDuplicate, file)
			return nil
		case TaskCreate:
			e.logOp(opUnlink, noteRevert, file)
			t.skip = true
			delete(e.plan.linkTaskFor, file)
			return nil
		}
	}
	// stow guards here with `$self->{dir_task_for}{$file} eq 'create'`, comparing
	// a hashref to a string. That is always false, so the guard never fires.
	// Ledger PL-08: dead code, not replicated.

	e.logOp(opUnlink, noteNone, file)
	source, err := os.Readlink(e.real(file))
	if err != nil {
		return fatalf("could not readlink %s (%v)", file, err)
	}
	t := &Task{Action: TaskRemove, Type: TypeLink, Path: file, Source: source}
	e.plan.tasks = append(e.plan.tasks, t)
	e.plan.linkTaskFor[file] = t
	return nil
}

func (e *engine) doMkdir(dir string) {
	if t, ok := e.plan.dirTaskFor[dir]; ok {
		switch t.Action {
		case TaskCreate:
			e.logOp(opMkdir, noteDuplicate, dir)
			return
		case TaskRemove:
			e.logOp(opMkdir, noteRevert, dir)
			t.skip = true
			delete(e.plan.dirTaskFor, dir)
			return
		}
	}

	e.logOp(opMkdir, noteNone, dir)
	t := &Task{Action: TaskCreate, Type: TypeDir, Path: dir}
	e.plan.tasks = append(e.plan.tasks, t)
	e.plan.dirTaskFor[dir] = t
}

// doRmdir prints without a colon; see the operation table above (ledger PL-05).
//
// stow's dir_task_for branch here reads the *link* task table and is unreachable
// dead code (ledger PL-06, probed): do_rmdir runs only from fold_tree during
// plan_unstow, which precedes plan_stow, so no dir task can exist for dir yet.
// The branch is therefore omitted rather than reproduced.
func (e *engine) doRmdir(dir string) {
	e.logOp(opRmdir, noteNone, dir)
	t := &Task{Action: TaskRemove, Type: TypeDir, Path: dir}
	e.plan.tasks = append(e.plan.tasks, t)
	e.plan.dirTaskFor[dir] = t
}

func (e *engine) doMv(src, dst string) {
	e.logOp(opMv, noteNone, fmt.Sprintf("%s -> %s", src, dst))
	e.plan.tasks = append(e.plan.tasks, &Task{Action: TaskMove, Type: TypeFile, Path: src, Dest: dst})
}

// processTasks executes the surviving tasks in plan order.
func (e *engine) processTasks() error {
	e.debug(2, 0, "Processing tasks...")
	var live []*Task
	for _, t := range e.plan.tasks {
		if !t.skip {
			live = append(live, t)
		}
	}
	e.plan.tasks = live
	if len(live) == 0 {
		return nil
	}
	for _, t := range live {
		if err := e.processTask(t); err != nil {
			return err
		}
	}
	e.debug(2, 0, "Processing tasks... done")
	return nil
}

func (e *engine) processTask(t *Task) error {
	switch {
	case t.Action == TaskCreate && t.Type == TypeDir:
		if err := os.Mkdir(e.real(t.Path), 0o777); err != nil {
			return fatalf("Could not create directory: %s (%v)", t.Path, err)
		}
	case t.Action == TaskCreate && t.Type == TypeLink:
		if err := os.Symlink(t.Source, e.real(t.Path)); err != nil {
			return fatalf("Could not create symlink: %s => %s (%v)", t.Path, t.Source, err)
		}
	case t.Action == TaskRemove && t.Type == TypeDir:
		if err := os.Remove(e.real(t.Path)); err != nil {
			return fatalf("Could not remove directory: %s (%v)", t.Path, err)
		}
	case t.Action == TaskRemove && t.Type == TypeLink:
		if err := os.Remove(e.real(t.Path)); err != nil {
			return fatalf("Could not remove link: %s (%v)", t.Path, err)
		}
	case t.Action == TaskMove && t.Type == TypeFile:
		if err := os.Rename(e.real(t.Path), e.real(t.Dest)); err != nil {
			return fatalf("Could not move %s -> %s (%v)", t.Path, t.Dest, err)
		}
	default:
		return fatalf("bad task action")
	}
	return nil
}

// debug mirrors Stow::Util::debug — four spaces per indent level, onto the log
// stream (stderr for the CLI). Levels 0-2 are a byte-exact contract; see PL-11.
func (e *engine) debug(level, indent int, format string, args ...any) {
	if e.opts.Verbosity < level {
		return
	}
	fmt.Fprintf(e.log, "%s%s\n", strings.Repeat("    ", indent), fmt.Sprintf(format, args...))
}
