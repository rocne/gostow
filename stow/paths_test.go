package stow

import "testing"

// Ported from stow 2.4.1's t/join_paths.t (22 assertions). stow's own suite is
// the only artefact that records *intent* rather than behaviour, so these cases
// catch a misreading that a differential test against the binary would not.
func TestJoinPaths(t *testing.T) {
	tests := []struct {
		scenario string
		in       []string
		want     string
	}{
		{"simple", []string{"a/b/c", "d/e/f"}, "a/b/c/d/e/f"},
		{"relative then absolute", []string{"a/b/c", "/d/e/f"}, "/d/e/f"},
		{"absolute then relative", []string{"/a/b/c", "d/e/f"}, "/a/b/c/d/e/f"},
		{"two absolutes", []string{"/a/b/c", "/d/e/f"}, "/d/e/f"},
		{"two absolutes with trailing /", []string{"/a/b/c/", "/d/e/f/"}, "/d/e/f"},
		{"multiple /'s, absolute", []string{"///a/b///c//", "/d///////e/f"}, "/d/e/f"},
		{"multiple /'s, relative", []string{"///a/b///c//", "d///////e/f"}, "/a/b/c/d/e/f"},
		{"first empty", []string{"", "a/b/c"}, "a/b/c"},
		{"second empty", []string{"a/b/c", ""}, "a/b/c"},
		{"first is /", []string{"/", "a/b/c"}, "/a/b/c"},
		{"second is /", []string{"a/b/c", "/"}, "/"},
		{"relative with ../", []string{"../a1/b1/../c1/", "a2/../b2/e2"}, "../a1/c1/b2/e2"},
		{"absolute with ../", []string{"../a1/b1/../c1/", "/a2/../b2/e2"}, "/b2/e2"},
		{"lots of ../", []string{"../a1/../../c1", "a2/../../"}, "../.."},
		{`drop any "./"`, []string{"./", "../a2"}, "../a2"},
		{`drop any "./foo"`, []string{"./a1", "../../a2"}, "../a2"},
		{". on RHS", []string{"a/b/c", "."}, "a/b/c"},
		{". in middle", []string{"a/b/c", ".", "d/e"}, "a/b/c/d/e"},
		{"0 at start", []string{"0", "a/b"}, "0/a/b"},
		{"/0 at start", []string{"/0", "a/b"}, "/0/a/b"},
		{"0 in middle", []string{"a/b/c", "0", "d/e"}, "a/b/c/0/d/e"},
		{"0 at end", []string{"a/b", "0"}, "a/b/0"},
	}
	for _, tt := range tests {
		t.Run(tt.scenario, func(t *testing.T) {
			if got := joinPaths(tt.in...); got != tt.want {
				t.Errorf("joinPaths(%q) = %q, want %q", tt.in, got, tt.want)
			}
		})
	}
}

// Ported from stow 2.4.1's t/parent.t (5 assertions).
func TestParent(t *testing.T) {
	tests := []struct {
		scenario string
		in       string
		want     string
	}{
		{"no leading or trailing /", "a/b/c", "a/b"},
		{"leading /", "/a/b/c", "/a/b"},
		{"trailing /", "a/b/c/", "a/b"},
		{"multiple /", "/////a///b///c///", "/a/b"},
		{"empty parent", "a", ""},
	}
	for _, tt := range tests {
		t.Run(tt.scenario, func(t *testing.T) {
			if got := parent(tt.in); got != tt.want {
				t.Errorf("parent(%q) = %q, want %q", tt.in, got, tt.want)
			}
		})
	}
}

// canonpath is a port of a Perl core routine and is exercised only indirectly by
// join_paths. These cases pin the transformations join_paths depends on, and
// the two places it differs from filepath.Clean: ".." survives, and a relative
// path never gains a leading "./".
func TestCanonpath(t *testing.T) {
	tests := []struct{ in, want string }{
		{"", ""},
		{"/", "/"},
		{"a////b", "a/b"},
		{"a/./b", "a/b"},
		{"a/././b", "a/b"},
		{"a/.", "a"},
		{"/.", "/"},
		{"./xx", "xx"},
		{"././xx", "xx"},
		{"./", "."}, // the `unless $path eq "./"` guard is undone by the trailing-slash strip
		{"/../../xx", "/xx"},
		{"/..", "/"},
		{"xx/", "xx"},
		{"a/../b", "a/../b"}, // NOT filepath.Clean's "b"
		{"../a", "../a"},
	}
	for _, tt := range tests {
		if got := canonpath(tt.in); got != tt.want {
			t.Errorf("canonpath(%q) = %q, want %q", tt.in, got, tt.want)
		}
	}
}

func TestRemoveParentRefs(t *testing.T) {
	tests := []struct{ in, want string }{
		{"a/../b", "b"},
		{"a/..", ""},
		{"/a/../b", "/b"},
		{"../..", "../.."},          // ".." segments are never cancelled
		{"../a/../b", "../b"},       // the leading ".." survives
		{"..foo/../b", "..foo/../b"} /* lookahead rejects any ".."-prefixed segment */,
		{"a/b/../../c", "c"},
		{"a/./../b", "a/b"}, // "." matches [^/]+, so it is the segment that gets cancelled
	}
	for _, tt := range tests {
		if got := removeParentRefs(tt.in); got != tt.want {
			t.Errorf("removeParentRefs(%q) = %q, want %q", tt.in, got, tt.want)
		}
	}
}
