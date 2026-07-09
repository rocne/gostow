// Command gostow is GNU Stow, reimplemented in Go as a single static binary.
//
// The symlink engine is not implemented yet. This entry point exists so the
// release pipeline builds, signs, and publishes a real artifact from day one;
// the conformance spec it will be built against lives in docs/SPEC.md.
package main

import (
	"fmt"
	"os"
)

// stowVersion is the GNU Stow release gostow is a conformance clone of.
// The spec IS stow's behaviour at this version — see docs/SPEC.md §1.
const stowVersion = "2.4.1"

// version is build metadata, overridden at release time via the goreleaser
// ldflag -X main.version (see .goreleaser/gostow.yaml). Commit and date are
// deliberately absent until the real CLI surfaces them: an unreferenced var
// is dead code, and `unused` rightly fails the build on it.
var version = "dev"

// versionLine reports gostow's own version, naming the stow release it clones.
//
// This deliberately breaks byte-parity with `stow --version`, which prints
// "stow (GNU Stow) version 2.4.1". Parity is owed to the behaviour scripts
// depend on, not to how the tool identifies itself. See docs/SPEC.md §4.4.
func versionLine(v string) string {
	return fmt.Sprintf("gostow %s (GNU Stow %s compatible)", v, stowVersion)
}

func main() {
	for _, arg := range os.Args[1:] {
		if arg == "-V" || arg == "--version" {
			fmt.Println(versionLine(version))
			return
		}
	}

	fmt.Fprintln(os.Stderr, "gostow: the stow engine is not implemented yet — see docs/SPEC.md")
	os.Exit(2)
}
