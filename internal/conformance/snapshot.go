package conformance

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

// Snapshot serialises root into a deterministic, line-per-entry form: dirs,
// files (with 4-digit octal mode and a sha256 of their content), and symlinks
// (with the verbatim os.Readlink target). Symlink targets are recorded as
// strings and never resolved, and the walk never follows them — stow's whole
// contract is which relative link it wrote, not where it points. Ordering is a
// plain byte sort on the relative path so it is stable and locale-independent.
func Snapshot(root string) (string, error) {
	type entry struct {
		rel  string
		line string
	}
	var entries []entry

	err := filepath.WalkDir(root, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if path == root {
			return nil
		}
		rel, err := filepath.Rel(root, path)
		if err != nil {
			return err
		}
		rel = filepath.ToSlash(rel)

		switch {
		case d.Type()&fs.ModeSymlink != 0:
			target, err := os.Readlink(path)
			if err != nil {
				return err
			}
			entries = append(entries, entry{rel, fmt.Sprintf("link %s -> %s", rel, target)})
		case d.IsDir():
			entries = append(entries, entry{rel, fmt.Sprintf("dir  %s", rel)})
		default:
			info, err := d.Info()
			if err != nil {
				return err
			}
			content, err := os.ReadFile(path)
			if err != nil {
				return err
			}
			sum := sha256.Sum256(content)
			entries = append(entries, entry{rel, fmt.Sprintf("file %s %04o %s",
				rel, info.Mode().Perm(), hex.EncodeToString(sum[:]))})
		}
		return nil
	})
	if err != nil {
		return "", err
	}

	sort.Slice(entries, func(i, j int) bool { return entries[i].rel < entries[j].rel })

	var b strings.Builder
	for _, e := range entries {
		b.WriteString(e.line)
		b.WriteByte('\n')
	}
	return b.String(), nil
}
