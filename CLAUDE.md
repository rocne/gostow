# CLAUDE.md

This file contains guidance for Claude when working in this repository.

This is a living document. As we discuss conventions, preferences, and project
decisions, relevant guidance should be added here.

## Repository

This is the `gostow` repository — **GNU Stow, reimplemented in Go**, shipped as a
single static binary (no Perl). The engine is consumed as a library by the sibling
tool `dstow` (separate repo).

## The Parity Mandate (gostow's prime directive)

From the user's perspective there is **no difference between gostow and GNU stow**:
every existing script, config, flag, and option behaves identically. The **only**
things gostow does not replicate are stow's bugs and accidental/unexpected behavior.

- `--dotfiles` (dot-prefix ↔ `.` translation) is fully included.
- gostow honors stow's own `.stowrc` and `.stow-local-ignore` / `.stow-global-ignore`
  files plus stow's built-in ignore defaults. (That is *stow's* format — gostow
  invents no config of its own; it is flags-only, like stow.)
- **The one additive liberty:** color on a TTY (real stow emits none; coloring a
  TTY cannot affect any script or pipe). Slogan: *byte-compatible on a pipe,
  prettier on a TTY.*
- **Pin a target stow version** as the conformance referent (a modern, recent,
  stable release, e.g. the 2.4.x line — confirm exact latest stable). The spec IS
  stow's behavior at that version.

Design as a **deep module**: narrow public API (`Stow` / `Unstow` / `Restow`)
hiding tree-folding, conflict detection, dot-prefix translation, and ignore
handling. `--no-folding` is a native stow flag → the engine takes `fold` as a
parameter (dstow drives it; gostow's CLI defaults to folding-on like stow).

## Project Philosophy

Like its sibling projects, gostow is **deliberately engineered to a higher standard
than its scale demands.** Half the goal is a real, installable tool; the other half
is an exercise in robust, industry-standard release/distribution engineering.

- Default toward the robust, real-world pattern a serious org would run, over the
  minimal solution that merely works.
- Prefer the path that teaches/exercises the mechanism over a black-box shortcut.
- Guard against cargo-culting: every addition must either teach a transferable
  pattern OR serve a real install/use. If neither, it's out.

## Branching Strategy

Trunk-based development:
- `main` — default branch, always stable.
- `feature/<name>` — human-authored feature branches off `main`, PR back into it.
- `feature/claude-<name>` — Claude-authored branches (the prefix makes it visually
  clear the branch was Claude's work).

All changes go to a feature branch and merge via PR — **never commit directly to
`main`.** Before every push, run `gh pr view` to confirm the PR is still open; if it
merged, cut a new branch/PR.

## Release Process (target)

Ride the same machinery as the sibling projects: release-please driven by
Conventional Commit history, delegating artifact build/sign/publish to the central
`rocne/release-ci` reusable workflow. GoReleaser builds linux+darwin × amd64+arm64.
(This must be set up as part of standing up the repo; mirror dot-dagger's
`.github/workflows/` and `release-please-config.json`.) PR titles are the source of
truth for release automation — enforce Conventional-Commit PR titles.

## CLI / Output Conventions

Carry the sibling projects' conventions, **subject to the parity mandate**:
- Excellent, unified input/output.
- Unified colorization (color-on-TTY only, per the mandate; respect NO_COLOR).
- Strict CLI best-practice hygiene.

The reusable *plumbing* (colored cobra help/usage templates, ~100 lines) may be
**copied** from dot-dagger's `internal/ui/cobra.go` — do NOT carry dotd's semantic
*vocabulary* (that's dotd's domain policy). Note: much output is dictated by stow
parity, so gostow's house-style latitude is small by design. Do not extract a shared
UI library yet — that waits until `dstow` is a second real consumer (Rule of Three).

## Validation steps

- `go build ./...`, `go vet ./...`, `go test ./...` before committing.
- **Conformance:** the symlink engine is validated test-first against real GNU stow's
  documented behavior at the pinned version. Treat stow's own test suite / manual as
  the conformance spec.

## Commit and Push Cadence

Commit and push fairly often; validate a good state first. Batch conceptually
related work into one branch/PR rather than many small PRs. Update an existing PR
rather than opening a new one.

## Documentation

Claude reference docs live in `.claude/docs/`. Start each session by reading
`CLAUDE.md` and the concept doc `gostow-dstow-concept-2026-07-09.md` (carried over
from dot-dagger). Defer other files until the task needs them.
