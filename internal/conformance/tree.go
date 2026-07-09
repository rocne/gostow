// Package conformance is the differential test harness described in
// docs/TEST-PLAN.md §3: declarative fixture trees, deterministic snapshots, and
// the machinery to run gostow and real stow 2.4.1 side by side. It is
// test-support only.
package conformance

import (
	"os"
	"path/filepath"
	"sort"
)

type NodeKind int

const (
	Dir NodeKind = iota
	File
	Symlink
)

// Node is one fixture entry. A zero Mode means "default": stow's own suite never
// depends on a specific mode except when it deliberately makes something
// unreadable, so the default keeps fixtures terse.
type Node struct {
	Kind    NodeKind
	Content string
	Target  string
	Mode    os.FileMode
}

// Tree is a fixture keyed by "/"-separated path relative to the root, with no
// leading slash.
type Tree map[string]Node

func F(content string) Node { return Node{Kind: File, Content: content} }

func D() Node { return Node{Kind: Dir} }

func L(target string) Node { return Node{Kind: Symlink, Target: target} }

// Materialize writes the tree under root. Paths are created in sorted order so a
// parent directory always precedes its children, and any missing parents are
// created first. Symlinks are written verbatim — the Target is never resolved,
// because stow's contract is the exact relative link it emits. Mode is applied
// last so a chmod-000 file is still written before it becomes unreadable.
func (t Tree) Materialize(root string) error {
	paths := make([]string, 0, len(t))
	for p := range t {
		paths = append(paths, p)
	}
	sort.Strings(paths)

	for _, p := range paths {
		node := t[p]
		abs := filepath.Join(root, filepath.FromSlash(p))
		if err := os.MkdirAll(filepath.Dir(abs), 0o755); err != nil {
			return err
		}
		switch node.Kind {
		case Dir:
			if err := os.MkdirAll(abs, 0o755); err != nil {
				return err
			}
		case File:
			if err := os.WriteFile(abs, []byte(node.Content), 0o644); err != nil {
				return err
			}
		case Symlink:
			if err := os.Symlink(node.Target, abs); err != nil {
				return err
			}
		}
		if node.Mode != 0 && node.Kind != Symlink {
			if err := os.Chmod(abs, node.Mode); err != nil {
				return err
			}
		}
	}
	return nil
}
