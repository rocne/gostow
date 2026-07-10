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

// Node is one fixture entry. A nil Mode means "leave the default".
//
// Mode is a pointer because 0 is a real mode — no permissions at all — and the
// mode a fixture most often needs to set. When it was a bare os.FileMode the
// zero value doubled as "unset", so `Mode: 0o000` chmodded nothing; a fixture
// asking for an unreadable directory silently got a readable one, and the test
// built on it passed without testing anything.
type Node struct {
	Kind    NodeKind
	Content string
	Target  string
	Mode    *os.FileMode
}

// Tree is a fixture keyed by "/"-separated path relative to the root, with no
// leading slash.
type Tree map[string]Node

func F(content string) Node { return Node{Kind: File, Content: content} }

func D() Node { return Node{Kind: Dir} }

func L(target string) Node { return Node{Kind: Symlink, Target: target} }

// Chmod returns a copy of n whose mode is set explicitly. Symlink nodes ignore it.
func (n Node) Chmod(mode os.FileMode) Node { n.Mode = &mode; return n }

// Materialize writes the tree under root. Paths are created in sorted order so a
// parent directory always precedes its children, and any missing parents are
// created first. Symlinks are written verbatim — the Target is never resolved,
// because stow's contract is the exact relative link it emits.
//
// Every mode is applied in a second pass, deepest path first. A single pass
// cannot work: "pkg/sub" sorts before "pkg/sub/f", so a chmod-000 directory would
// be sealed before the file inside it was written, and a fixture could not
// describe an unreadable directory with contents. Chmodding deepest-first means
// sealing a directory never blocks the chmod of something beneath it.
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
	}

	for i := len(paths) - 1; i >= 0; i-- {
		node := t[paths[i]]
		if node.Mode == nil || node.Kind == Symlink {
			continue
		}
		if err := os.Chmod(filepath.Join(root, filepath.FromSlash(paths[i])), *node.Mode); err != nil {
			return err
		}
	}
	return nil
}
