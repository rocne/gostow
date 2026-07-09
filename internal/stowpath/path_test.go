package stowpath

import "testing"

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

		// The cases the CLI's deleted copy got wrong. Perl's split leaves a
		// leading empty field, which is dropped along with the last segment, so a
		// single-segment absolute path has parent "" — not "/". bin/stow then
		// substitutes ".", making `stow -d /tmp pkg` target the cwd. The copy
		// returned "/" and aimed the symlink farm at the filesystem root.
		{"single segment, absolute", "/tmp", ""},
		{"single segment, absolute, trailing /", "/tmp/", ""},
		{"single segment, absolute, doubled /", "//tmp", ""},
		{"root", "/", ""},
		{"empty", "", ""},
		{"two segments, absolute", "/tmp/x", "/tmp"},
	}
	for _, tt := range tests {
		t.Run(tt.scenario, func(t *testing.T) {
			if got := Parent(tt.in); got != tt.want {
				t.Errorf("Parent(%q) = %q, want %q", tt.in, got, tt.want)
			}
		})
	}
}
