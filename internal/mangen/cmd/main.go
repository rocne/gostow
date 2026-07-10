// Command mangen rewrites the DIVERGENCES section of man/gostow.8 from
// docs/DIVERGENCES.md. It is wired to `go generate ./...` (see mangen.go).
package main

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/rocne/gostow/internal/mangen"
)

func main() {
	if err := run(); err != nil {
		fmt.Fprintln(os.Stderr, "mangen:", err)
		os.Exit(1)
	}
}

func run() error {
	cwd, err := os.Getwd()
	if err != nil {
		return err
	}
	root, err := mangen.RepoRoot(cwd)
	if err != nil {
		return err
	}

	docPath := filepath.Join(root, mangen.DocPath)
	manPath := filepath.Join(root, mangen.ManPath)

	doc, err := os.ReadFile(docPath)
	if err != nil {
		return err
	}
	man, err := os.ReadFile(manPath)
	if err != nil {
		return err
	}

	body, err := mangen.Render(string(doc))
	if err != nil {
		return err
	}
	next, err := mangen.Splice(string(man), body)
	if err != nil {
		return err
	}

	if next == string(man) {
		fmt.Println("mangen: man/gostow.8 already up to date")
		return nil
	}
	if err := os.WriteFile(manPath, []byte(next), 0o644); err != nil {
		return err
	}
	fmt.Println("mangen: regenerated the DIVERGENCES section of man/gostow.8")
	return nil
}
