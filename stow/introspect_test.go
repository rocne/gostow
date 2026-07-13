package stow

import (
	"os"
	"path/filepath"
	"reflect"
	"sort"
	"testing"
)

// symlink creates a symlink at path pointing at dest, making parents as needed.
func symlink(t *testing.T, dest, path string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(dest, path); err != nil {
		t.Fatal(err)
	}
}

// --- Owner (#35) ---------------------------------------------------------

func TestOwnerReportsStowingPackage(t *testing.T) {
	root := sandbox(t)
	write(t, filepath.Join(root, "stow/pkg/f"), "x")
	if _, err := Stow(opts(root), "pkg"); err != nil {
		t.Fatalf("Stow: %v", err)
	}

	pkg, owned, err := Owner(filepath.Join(root, "stow"), filepath.Join(root, "target/f"))
	if err != nil {
		t.Fatalf("Owner: %v", err)
	}
	if !owned || pkg != "pkg" {
		t.Errorf("Owner = (%q, %v), want (\"pkg\", true)", pkg, owned)
	}
}

func TestOwnerRejectsUnownedLink(t *testing.T) {
	root := sandbox(t)
	// A link into the target itself, owned by no stow package.
	write(t, filepath.Join(root, "target/real"), "x")
	symlink(t, "real", filepath.Join(root, "target/l"))

	pkg, owned, err := Owner(filepath.Join(root, "stow"), filepath.Join(root, "target/l"))
	if err != nil {
		t.Fatalf("Owner: %v", err)
	}
	if owned || pkg != "" {
		t.Errorf("Owner = (%q, %v), want (\"\", false)", pkg, owned)
	}
}

func TestOwnerRejectsAbsoluteLink(t *testing.T) {
	root := sandbox(t)
	symlink(t, filepath.Join(root, "stow/pkg/f"), filepath.Join(root, "target/l"))

	pkg, owned, err := Owner(filepath.Join(root, "stow"), filepath.Join(root, "target/l"))
	if err != nil {
		t.Fatalf("Owner: %v", err)
	}
	if owned || pkg != "" {
		t.Errorf("Owner = (%q, %v), want (\"\", false): stow never owns an absolute link", pkg, owned)
	}
}

func TestOwnerAttributesToTheRightPackage(t *testing.T) {
	root := sandbox(t)
	write(t, filepath.Join(root, "stow/a/x"), "1")
	write(t, filepath.Join(root, "stow/b/y"), "2")
	if _, err := Stow(opts(root), "a", "b"); err != nil {
		t.Fatalf("Stow: %v", err)
	}
	for _, tc := range []struct{ link, want string }{{"target/x", "a"}, {"target/y", "b"}} {
		pkg, owned, err := Owner(filepath.Join(root, "stow"), filepath.Join(root, tc.link))
		if err != nil {
			t.Fatalf("Owner(%s): %v", tc.link, err)
		}
		if !owned || pkg != tc.want {
			t.Errorf("Owner(%s) = (%q, %v), want (%q, true)", tc.link, pkg, owned, tc.want)
		}
	}
}

func TestOwnerHonoursMarkedStowDir(t *testing.T) {
	root := sandbox(t)
	// A stow dir marked with .stow, elsewhere than the queried dir, reached by a
	// link that climbs out of the target. find_stowed_path's marker walk owns it.
	write(t, filepath.Join(root, "other/pkg/f"), "x")
	write(t, filepath.Join(root, "other/.stow"), "")
	symlink(t, "../other/pkg/f", filepath.Join(root, "target/f"))

	pkg, owned, err := Owner(filepath.Join(root, "stow"), filepath.Join(root, "target/f"))
	if err != nil {
		t.Fatalf("Owner: %v", err)
	}
	if !owned || pkg != "pkg" {
		t.Errorf("Owner = (%q, %v), want (\"pkg\", true)", pkg, owned)
	}
}

// --- DefaultIgnores + NoGlobalIgnoreFile (#36) ---------------------------

func TestDefaultIgnoresCarriesStowsBuiltins(t *testing.T) {
	got := DefaultIgnores()
	want := map[string]bool{`\.git`: false, `RCS`: false, `^/\.stow-local-ignore$`: false}
	for _, p := range got {
		if _, ok := want[p]; ok {
			want[p] = true
		}
	}
	for p, seen := range want {
		if !seen {
			t.Errorf("DefaultIgnores() missing built-in pattern %q; got %v", p, got)
		}
	}
}

func TestDefaultIgnoresReturnsFreshSlice(t *testing.T) {
	a := DefaultIgnores()
	if len(a) == 0 {
		t.Fatal("DefaultIgnores() is empty")
	}
	a[0] = "MUTATED"
	if b := DefaultIgnores(); b[0] == "MUTATED" {
		t.Error("DefaultIgnores() shares mutable state between calls")
	}
}

func TestNoGlobalIgnoreFileSuppressesTheHomeRead(t *testing.T) {
	root := sandbox(t)
	// A global ignore file replaces the built-in defaults with just "foo".
	write(t, filepath.Join(root, "home/.stow-global-ignore"), "foo\n")
	write(t, filepath.Join(root, "stow/pkg/foo"), "1")
	write(t, filepath.Join(root, "stow/pkg/bar"), "2")

	o := opts(root)
	withGlobal, err := Expected(o, "pkg")
	if err != nil {
		t.Fatalf("Expected: %v", err)
	}
	if _, ok := withGlobal["foo"]; ok {
		t.Errorf("with the global ignore file, foo should be ignored; got %v", withGlobal)
	}
	if _, ok := withGlobal["bar"]; !ok {
		t.Errorf("bar should not be ignored; got %v", withGlobal)
	}

	o.NoGlobalIgnoreFile = true
	suppressed, err := Expected(o, "pkg")
	if err != nil {
		t.Fatalf("Expected: %v", err)
	}
	if _, ok := suppressed["foo"]; !ok {
		t.Errorf("with NoGlobalIgnoreFile, foo must not be ignored; got %v", suppressed)
	}
}

// --- Expected (#33) ------------------------------------------------------

func TestExpectedFoldsTopLevelDirectories(t *testing.T) {
	root := sandbox(t)
	write(t, filepath.Join(root, "stow/pkg/f"), "x")
	write(t, filepath.Join(root, "stow/pkg/d/a"), "y")
	write(t, filepath.Join(root, "stow/pkg/d/b"), "z")

	got, err := Expected(opts(root), "pkg")
	if err != nil {
		t.Fatalf("Expected: %v", err)
	}
	want := map[string]string{"f": "f", "d": "d"} // d folds whole; not descended
	if !reflect.DeepEqual(got, want) {
		t.Errorf("Expected = %v, want %v", got, want)
	}
}

func TestExpectedWithoutFoldingDescends(t *testing.T) {
	root := sandbox(t)
	write(t, filepath.Join(root, "stow/pkg/f"), "x")
	write(t, filepath.Join(root, "stow/pkg/d/a"), "y")
	write(t, filepath.Join(root, "stow/pkg/d/b"), "z")

	o := opts(root)
	o.Fold = false
	got, err := Expected(o, "pkg")
	if err != nil {
		t.Fatalf("Expected: %v", err)
	}
	want := map[string]string{"f": "f", "d/a": "d/a", "d/b": "d/b"} // d itself: shape only
	if !reflect.DeepEqual(got, want) {
		t.Errorf("Expected = %v, want %v", got, want)
	}
}

func TestExpectedTranslatesDotfiles(t *testing.T) {
	root := sandbox(t)
	write(t, filepath.Join(root, "stow/pkg/dot-bashrc"), "x")

	o := opts(root)
	o.Dotfiles = true
	got, err := Expected(o, "pkg")
	if err != nil {
		t.Fatalf("Expected: %v", err)
	}
	want := map[string]string{".bashrc": "dot-bashrc"}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("Expected = %v, want %v", got, want)
	}
}

func TestExpectedHonoursIgnoreAndAbsoluteSymlink(t *testing.T) {
	root := sandbox(t)
	write(t, filepath.Join(root, "stow/pkg/keep"), "x")
	write(t, filepath.Join(root, "stow/pkg/.git"), "should be ignored by defaults")
	symlink(t, "/etc/hostname", filepath.Join(root, "stow/pkg/abs"))

	got, err := Expected(opts(root), "pkg")
	if err != nil {
		t.Fatalf("Expected: %v", err)
	}
	want := map[string]string{"keep": "keep"} // .git ignored; abs symlink unrepresentable
	if !reflect.DeepEqual(got, want) {
		t.Errorf("Expected = %v, want %v", got, want)
	}
}

// TestExpectedMatchesEmptyTargetStow binds the plan-only walk to the real engine:
// stowing into an empty target must create exactly the links Expected predicts.
// A drift between the two implementations fails here, with no oracle required.
func TestExpectedMatchesEmptyTargetStow(t *testing.T) {
	for _, fold := range []bool{true, false} {
		t.Run(map[bool]string{true: "fold", false: "no-fold"}[fold], func(t *testing.T) {
			root := sandbox(t)
			write(t, filepath.Join(root, "stow/pkg/f"), "x")
			write(t, filepath.Join(root, "stow/pkg/d/a"), "y")
			write(t, filepath.Join(root, "stow/pkg/d/sub/c"), "z")
			write(t, filepath.Join(root, "stow/pkg/dot-config/nvim"), "w")

			o := opts(root)
			o.Fold = fold
			o.Dotfiles = true
			o.Simulate = true

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
		})
	}
}

// --- Structured conflicts (#34) ------------------------------------------

// conflictsOf plans a stow of pkg and returns the collected conflicts.
func conflictsOf(t *testing.T, o Options, pkg string) []Conflict {
	t.Helper()
	res, err := Stow(o, pkg)
	if err == nil {
		t.Fatalf("Stow(%s) unexpectedly succeeded", pkg)
	}
	return res.Conflicts
}

func TestConflictExposesPathAndKind(t *testing.T) {
	tests := []struct {
		name  string
		setup func(root string)
		adopt bool
		path  string
		kind  ConflictKind
	}{
		{
			name: "existing file",
			setup: func(root string) {
				write(t, filepath.Join(root, "stow/pkg/f"), "new")
				write(t, filepath.Join(root, "target/f"), "old")
			},
			path: "f", kind: ConflictExistingFile,
		},
		{
			name: "non-directory over directory",
			setup: func(root string) {
				write(t, filepath.Join(root, "stow/pkg/d"), "a file named d")
				if err := os.MkdirAll(filepath.Join(root, "target/d"), 0o755); err != nil {
					t.Fatal(err)
				}
			},
			path: "d", kind: ConflictDirMismatch,
		},
		{
			name: "foreign link",
			setup: func(root string) {
				write(t, filepath.Join(root, "stow/pkg/f"), "new")
				write(t, filepath.Join(root, "target/real"), "x")
				symlink(t, "real", filepath.Join(root, "target/f"))
			},
			path: "f", kind: ConflictForeignLink,
		},
		{
			name: "absolute symlink source",
			setup: func(root string) {
				symlink(t, "/etc/hostname", filepath.Join(root, "stow/pkg/f"))
			},
			path: "f", kind: ConflictSourceAbsolute,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			root := sandbox(t)
			tt.setup(root)
			o := opts(root)
			o.Adopt = tt.adopt
			cs := conflictsOf(t, o, "pkg")
			if len(cs) != 1 {
				t.Fatalf("got %d conflicts, want 1: %+v", len(cs), cs)
			}
			if cs[0].Path != tt.path || cs[0].Kind != tt.kind {
				t.Errorf("Conflict{Path:%q, Kind:%v}, want {Path:%q, Kind:%v}; msg=%q",
					cs[0].Path, cs[0].Kind, tt.path, tt.kind, cs[0].Message)
			}
		})
	}
}

func TestConflictOtherPackageKind(t *testing.T) {
	root := sandbox(t)
	write(t, filepath.Join(root, "stow/a/f"), "1")
	write(t, filepath.Join(root, "stow/b/f"), "2")
	if _, err := Stow(opts(root), "a"); err != nil {
		t.Fatalf("Stow a: %v", err)
	}
	cs := conflictsOf(t, opts(root), "b")
	if len(cs) != 1 {
		t.Fatalf("got %d conflicts, want 1: %+v", len(cs), cs)
	}
	if cs[0].Path != "f" || cs[0].Kind != ConflictOtherPackage {
		t.Errorf("Conflict{Path:%q, Kind:%v}, want {Path:\"f\", Kind:%v}; msg=%q",
			cs[0].Path, cs[0].Kind, ConflictOtherPackage, cs[0].Message)
	}
}
