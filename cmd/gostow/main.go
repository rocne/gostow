// Command gostow is GNU Stow, reimplemented in Go as a single static binary.
//
// From a consumer's perspective — a script, a .stowrc, a pipe, a $? check —
// gostow and GNU Stow 2.4.1 are indistinguishable. The conformance spec, and the
// ledger of every place stow's behaviour is a bug rather than a contract, live
// in docs/SPEC.md.
package main

import (
	"os"
	"runtime/debug"

	"github.com/rocne/gostow/internal/cli"
)

// version is build metadata, overridden at release time via the goreleaser
// ldflag -X main.version (see .goreleaser/gostow.yaml).
var version = "dev"

// devVersion is what version holds when nobody set the ldflag.
const devVersion = "dev"

// goModuleDevVersion is what the toolchain records for a binary built from a
// working tree rather than fetched as a module.
const goModuleDevVersion = "(devel)"

func main() {
	os.Exit(cli.Run(os.Args, resolveVersion(version, debug.ReadBuildInfo), os.Stdout, os.Stderr))
}

// resolveVersion answers the only question --version has to get right: which
// gostow is this?
//
// Two build paths, and only one of them sets the ldflag:
//
//   - goreleaser passes -X main.version=v0.1.0, so ldflag wins.
//   - `go install github.com/rocne/gostow/cmd/gostow@v0.1.0` passes nothing, and
//     the binary would report "dev" — a lie, since the toolchain knows exactly
//     which module version it fetched and stamps it into the build info.
//
// So fall back to the module version, and fall back again to "dev" when even
// that is unknown: a build from a working tree records "(devel)", which is no
// more informative than "dev" and much stranger to read.
//
// readBuildInfo is a parameter so the fallback is testable; it is always
// debug.ReadBuildInfo in production.
func resolveVersion(ldflagVersion string, readBuildInfo func() (*debug.BuildInfo, bool)) string {
	if ldflagVersion != devVersion && ldflagVersion != "" {
		return ldflagVersion
	}
	if bi, ok := readBuildInfo(); ok {
		if v := bi.Main.Version; v != "" && v != goModuleDevVersion {
			return v
		}
	}
	return devVersion
}
