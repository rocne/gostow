// Command gostow is GNU Stow, reimplemented in Go as a single static binary.
//
// From a consumer's perspective — a script, a .stowrc, a pipe, a $? check —
// gostow and GNU Stow 2.4.1 are indistinguishable. The conformance spec, and the
// ledger of every place stow's behaviour is a bug rather than a contract, live
// in docs/SPEC.md.
package main

import (
	"os"

	"github.com/rocne/gostow/internal/cli"
)

// version is build metadata, overridden at release time via the goreleaser
// ldflag -X main.version (see .goreleaser/gostow.yaml).
var version = "dev"

func main() {
	os.Exit(cli.Run(os.Args, version, os.Stdout, os.Stderr))
}
