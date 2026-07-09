package conformance

import (
	"bytes"
	"errors"
	"os/exec"
)

type Run struct {
	Stdout, Stderr string
	ExitCode       int
	Tree           string
}

// RunBinary executes bin and captures its streams and exit code. A nonzero exit
// is data, not an error, so it is never surfaced as one; only a binary that
// could not be started yields the 127 sentinel.
//
// env replaces the environment wholesale — parity work must control exactly what
// stow sees. A nil env means an *empty* environment, not an inherited one: Go's
// exec treats nil Env as "inherit", which would let the developer's ~/.stowrc
// and ~/.stow-global-ignore leak into the fixture and silently rewrite the spec.
func RunBinary(bin string, args []string, env []string, cwd string) Run {
	cmd := exec.Command(bin, args...)
	if env == nil {
		env = []string{}
	}
	cmd.Env = env
	cmd.Dir = cwd

	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	err := cmd.Run()
	run := Run{Stdout: stdout.String(), Stderr: stderr.String()}

	var exitErr *exec.ExitError
	switch {
	case err == nil:
		run.ExitCode = 0
	case errors.As(err, &exitErr):
		run.ExitCode = exitErr.ExitCode()
	default:
		run.ExitCode = 127
	}
	return run
}
