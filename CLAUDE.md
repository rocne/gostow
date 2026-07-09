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

## The pinned conformance referent

**GNU Stow 2.4.1** (2024-09-08). The spec IS stow's behavior at this version.

Real stow 2.4.1 is an **executable oracle** — this project's specification can be
*run*. Install it with `test/install-stow-oracle.sh`, which builds the
checksum-pinned tarball from source. Do **not** `apt install stow`: Ubuntu 24.04
ships 2.3.1, which would silently redefine the spec.

When this document, `docs/SPEC.md`, and the real binary disagree, **the binary wins.**

## Validation steps

- `go build ./...`, `go vet ./...`, `go test ./...` before committing.
  (Note `go build ./...` drops a `gostow` binary at the repo root; it's gitignored.)
- `golangci-lint run ./...` — `unused` will fail the build on an unreferenced
  package-level var, so don't add ldflags targets before something reads them.
- `actionlint` before touching `.github/workflows/`.
- **Conformance:** `go test ./...` is hermetic (goldens, no Perl).
  `go test -tags oracle ./...` runs the differential suite against real stow.

## Versioning — gostow stays pre-1.0

**A `v1` tag is a promise of stow parity we have not earned.** Until the engine is
complete, every release is `v0.x`. Four layers enforce this; don't weaken any of
them without a deliberate decision:

- `initial-version: 0.1.0` in `release-please-config.json`. Without it release-please
  proposes **1.0.0** on a first release — `bump-minor-pre-major` does not prevent this,
  because the initial-release path never bumps anything.
- `guard-release-pr` in `release-please.yml`. `ci.yml`'s `version-guard` **cannot** see
  release-please's own PR: GitHub never triggers `pull_request` workflows for
  `GITHUB_TOKEN`-authored PRs.
- `version-guard` in `ci.yml`, for human PRs touching the manifest.
- `guard-pre-1-0` in both release workflows — the hard stop before anything is published.

## Commit and Push Cadence

Commit and push fairly often; validate a good state first. Batch conceptually
related work into one branch/PR rather than many small PRs. Update an existing PR
rather than opening a new one.

## Documentation

Start each session by reading, in order:

1. `CLAUDE.md` (this file).
2. **`docs/SPEC.md`** — the conformance spec. Every claim is tagged `[probed]`
   (verified by running real stow), `[source]` (read from stow's Perl), or
   `[inferred]` (deduced, **not yet verified — do not rely on it**).
   §10 is the **Parity Ledger**: every stow quirk, its tier, and its ruling.
3. **`docs/TEST-PLAN.md`** — seams (S1/S2/S3, signed off), the differential-oracle +
   goldens strategy, and §5's vertical slice order. Build in that order.

Defer `.claude/docs/gostow-dstow-concept-2026-07-09.md` (design capture, carried over
from dot-dagger) until the task needs it — `docs/SPEC.md` supersedes it on anything
concrete.

### Current state (2026-07-09)

Release pipeline is live and green; `docs/SPEC.md` and `docs/TEST-PLAN.md` are written.
**No engine exists.** `cmd/gostow` answers `--version` and exits 2 on anything else.

Owed before implementation:

- **Probe PL-06, PL-09, PL-10** against the real binary. They are `[inferred]` from
  reading the Perl, and their tier — and therefore whether we replicate them — is
  undecided until probed.
- **Slice 3 (`internal/getopt`) is load-bearing and easy to underestimate.**
  `Getopt::Long`'s `bundling` + `permute` + `no_ignore_case` + `auto_abbrev` +
  `-v:+` semantics cannot be expressed with `pflag`/`cobra`. Every later CLI slice
  depends on it. See `docs/SPEC.md` §4.1.
- **PL-11** (byte-parity scope at verbosity ≥3) still needs a ruling.

`v0.1.0` would publish a stub whose `--version` claims `(GNU Stow 2.4.1 compatible)`.
release-please's standing release PR is deliberately left open until that is true.
