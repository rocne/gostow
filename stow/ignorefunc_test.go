package stow

import (
	"os"
	"path/filepath"
	"reflect"
	"sort"
	"testing"
)

// --- Options.IgnoreFunc (#41) ---------------------------------------------

func exists(t *testing.T, path string) bool {
	t.Helper()
	_, err := os.Lstat(path)
	if err != nil && !os.IsNotExist(err) {
		t.Fatal(err)
	}
	return err == nil
}

func TestIgnoreFuncExcludesFilesAndSubtrees(t *testing.T) {
	root := sandbox(t)
	write(t, filepath.Join(root, "stow/pkg/keep"), "k")
	write(t, filepath.Join(root, "stow/pkg/drop"), "d")
	write(t, filepath.Join(root, "stow/pkg/dropdir/inner"), "i")

	o := opts(root)
	o.Fold = false // descend everywhere, so the subtree cut is the func's doing
	o.IgnoreFunc = func(rel string, isDir bool) bool {
		return rel == "drop" || rel == "dropdir"
	}
	if _, err := Stow(o, "pkg"); err != nil {
		t.Fatalf("Stow: %v", err)
	}

	if !exists(t, filepath.Join(root, "target/keep")) {
		t.Error("keep was not stowed")
	}
	if exists(t, filepath.Join(root, "target/drop")) {
		t.Error("drop was stowed despite IgnoreFunc")
	}
	// Excluding the directory excludes its subtree: no dir, no descent.
	if exists(t, filepath.Join(root, "target/dropdir")) {
		t.Error("dropdir was created despite IgnoreFunc")
	}
}

// rel is the package-relative path as it exists on disk — before --dotfiles
// translation — and isDir is the package node's kind by lstat, so a symlink
// reports false. Both are pinned by recording every consultation.
func TestIgnoreFuncSeesPreTranslationPathsAndKinds(t *testing.T) {
	root := sandbox(t)
	write(t, filepath.Join(root, "stow/pkg/dot-config/nvim/init.lua"), "x")
	write(t, filepath.Join(root, "stow/pkg/plain"), "y")
	symlink(t, "plain", filepath.Join(root, "stow/pkg/alias"))

	o := opts(root)
	o.Fold = false
	o.Dotfiles = true
	seen := map[string]bool{}
	o.IgnoreFunc = func(rel string, isDir bool) bool {
		seen[rel] = isDir
		return false
	}
	if _, err := Stow(o, "pkg"); err != nil {
		t.Fatalf("Stow: %v", err)
	}

	want := map[string]bool{
		"dot-config":                true,  // not ".config": pre-translation
		"dot-config/nvim":           true,
		"dot-config/nvim/init.lua":  false,
		"plain":                     false,
		"alias":                     false, // a symlink is not a directory
	}
	if !reflect.DeepEqual(seen, want) {
		t.Errorf("IgnoreFunc consultations = %v, want %v", seen, want)
	}
	if !exists(t, filepath.Join(root, "target/.config/nvim/init.lua")) {
		t.Error("the translated tree was not stowed")
	}
}

// The combination is additive-only: returning false everywhere cannot
// resurrect a node the built-in machinery ignores. `.gitignore` is on stow's
// compiled-in default list, so with a permissive IgnoreFunc it must still be
// skipped — and the func must never even be asked about a node the built-ins
// already rejected, or a panicking caller could observe engine internals.
func TestIgnoreFuncCannotUnignore(t *testing.T) {
	root := sandbox(t)
	write(t, filepath.Join(root, "stow/pkg/f"), "x")
	write(t, filepath.Join(root, "stow/pkg/.gitignore"), "g")

	o := opts(root)
	var asked []string
	o.IgnoreFunc = func(rel string, isDir bool) bool {
		asked = append(asked, rel)
		return false
	}
	if _, err := Stow(o, "pkg"); err != nil {
		t.Fatalf("Stow: %v", err)
	}

	if exists(t, filepath.Join(root, "target/.gitignore")) {
		t.Error(".gitignore was stowed: IgnoreFunc must not be able to un-ignore")
	}
	if want := []string{"f"}; !reflect.DeepEqual(asked, want) {
		t.Errorf("IgnoreFunc was consulted for %v, want %v (built-in rejections are not re-asked)", asked, want)
	}
}

// Unstow consults the same seam: a node the func excludes is invisible to the
// walk, so its deployed link survives an unstow — the exact behaviour stow's
// own ignore machinery has, extended to the caller's patterns.
func TestIgnoreFuncAppliesToUnstow(t *testing.T) {
	root := sandbox(t)
	write(t, filepath.Join(root, "stow/pkg/gone"), "a")
	write(t, filepath.Join(root, "stow/pkg/kept"), "b")
	if _, err := Stow(opts(root), "pkg"); err != nil {
		t.Fatalf("Stow: %v", err)
	}

	o := opts(root)
	o.IgnoreFunc = func(rel string, isDir bool) bool { return rel == "kept" }
	if _, err := Unstow(o, "pkg"); err != nil {
		t.Fatalf("Unstow: %v", err)
	}

	if exists(t, filepath.Join(root, "target/gone")) {
		t.Error("gone should have been unstowed")
	}
	if !exists(t, filepath.Join(root, "target/kept")) {
		t.Error("kept was unstowed despite IgnoreFunc")
	}
}

// Expected and a real empty-target stow must agree under the same IgnoreFunc,
// exactly as they must without one — deterministic input (rel + kind) is what
// makes that promise keepable, and this binds it.
func TestIgnoreFuncKeepsExpectedAndStowConsistent(t *testing.T) {
	for _, fold := range []bool{true, false} {
		t.Run(map[bool]string{true: "fold", false: "no-fold"}[fold], func(t *testing.T) {
			root := sandbox(t)
			write(t, filepath.Join(root, "stow/pkg/f"), "x")
			write(t, filepath.Join(root, "stow/pkg/d/a"), "y")
			write(t, filepath.Join(root, "stow/pkg/d/skip/c"), "z")
			write(t, filepath.Join(root, "stow/pkg/dot-config/nvim"), "w")

			o := opts(root)
			o.Fold = fold
			o.Dotfiles = true
			o.Simulate = true
			o.IgnoreFunc = func(rel string, isDir bool) bool {
				return rel == "d/skip" || rel == "dot-config/nvim"
			}

			exp, err := Expected(o, "pkg")
			if err != nil {
				t.Fatalf("Expected: %v", err)
			}
			res, err := Stow(o, "pkg")
			if err != nil {
				t.Fatalf("Stow: %v", err)
			}

			var expKeys, linkPaths []string
			for k := range exp {
				expKeys = append(expKeys, k)
			}
			for _, task := range res.Tasks {
				if task.Action == TaskCreate && task.Type == TypeLink {
					linkPaths = append(linkPaths, task.Path)
				}
			}
			sort.Strings(expKeys)
			sort.Strings(linkPaths)
			if !reflect.DeepEqual(expKeys, linkPaths) {
				t.Errorf("Expected keys %v != empty-target link tasks %v", expKeys, linkPaths)
			}
			for _, k := range expKeys {
				if k == "d/skip" || k == ".config/nvim" || k == "d/skip/c" {
					t.Errorf("Expected contains %s, which IgnoreFunc excludes", k)
				}
			}
		})
	}
}
