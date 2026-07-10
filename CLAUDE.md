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
- **Help *prose* is not part of parity** (SPEC §4.5). The mandate covers scripts,
  configs, flags and options — option *parsing* stays byte-exact, and so does the
  usage diagnostic on stderr. But `--help`'s wording is gostow's own: stow is
  GPLv3 and gostow is MIT, and the copied block told users to mail their gostow
  bugs to `bug-stow@gnu.org`. Ignore patterns and error messages *are* behaviour
  and stay verbatim; see `NOTICE`.
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

**Squash-merge only.** The repo allows no other method, the squash subject is the PR
title, and the squash body is the PR body. This is what makes "PR titles are the source
of truth" true rather than aspirational: a merge commit whose subject is also the PR
title hands release-please the same Conventional Commit twice, and the changelog says
so. PR #18 shipped that duplicate into 0.1.1's changelog. A `Release-As:` or
`BREAKING CHANGE:` footer therefore belongs in the **PR body**, which is the only route
by which a footer now reaches `main`.

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
  (Build the binary with `go build -o gostow ./cmd/gostow`; it's gitignored.)
- `golangci-lint run ./...` — `unused` will fail the build on an unreferenced
  package-level var, so don't add ldflags targets before something reads them.
- `actionlint` before touching `.github/workflows/`.
- **Conformance:** `go test ./...` is hermetic (goldens, no Perl).
  `go test -tags oracle ./...` runs the differential suite against real stow.

### The test cache cannot see the oracle — do not trust a `(cached)` result

`go test` validates a cached result against the test binary, its arguments, the
environment variables the test read, and the files it opened **through package `os`**.
Every oracle here lives in a *subprocess*: real `stow`, real `perl`, `Stow.pm`,
`Getopt::Long`, and the two `.pl` driver scripts. None of that is visible to the cache.

Measured, not theorised: replacing `Stow.pm`'s `ignore()` with `return 0` and re-running
`go test -tags oracle ./stow/` printed `ok (cached)` — 1216 verdicts "verified" against an
oracle answering *no* to everything. The same held for both `.pl` scripts, and for gostow's
own source before `internal/conformance` gained an import edge to `internal/cli`.

Two defences, both required:

- `conformance.TrackOracleInput(t, paths...)` reads each subprocess input so its content
  hash enters the cache key. **Call it whenever you add an oracle input.**
- CI passes `-count=1`. Use it yourself when a result matters.

And: **a conformance test that skips is a vacuous pass.** `OraclePath`, `RequirePerl` and
`OraclePerlLib` all `t.Fatal`. Never soften one to `t.Skip` — with no oracle installed the
old `t.Skip` made the whole differential suite print `ok` in 0.26s instead of 5.8s.

## Versioning — gostow stays pre-1.0

**A `v1` tag is a promise of stow parity we have not earned.** Until the engine is
complete, every release is `v0.x`. Don't weaken any of these without a deliberate
decision:

- `bump-minor-pre-major: true` in `release-please-config.json`. Below 1.0.0 a breaking
  change bumps the **minor**, so ordinary commit traffic can never cross the line.
- `guard-release-pr` in `release-please.yml`. `ci.yml`'s `version-guard` **cannot** see
  release-please's own PR: GitHub never triggers `pull_request` workflows for
  `GITHUB_TOKEN`-authored PRs.
- `version-guard` in `ci.yml`, for human PRs touching the manifest.
- `guard-pre-1-0` in both release workflows — the hard stop before anything is published.

`initial-version: 0.1.0` is **spent.** It governs only the *first* release, and without it
release-please would have proposed 1.0.0 for that one — `bump-minor-pre-major` does not
prevent it, because the initial-release path never bumps anything. `v0.1.0` shipped on
2026-07-09, so that path is closed forever; the setting stays as a record of why, not as a
guard. The corollary: **reaching 1.0.0 now takes a deliberate act** — a `Release-As: 1.0.0`
footer, or dropping `bump-minor-pre-major` plus a breaking commit. PR #13 was written
against the old lever and needs rewriting.

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

### Current state (2026-07-10)

**`v0.1.1` is released and published** — tag, GitHub release, Cloudsmith `.deb`/`.rpm`,
Homebrew cask, sigstore attestation. It carries every fix from the 2026-07-10 audit. The
release pipeline is live and green, and **the engine and CLI are implemented**: `gostow`
stows, unstows, restows, folds, unfolds and refolds; parses `.stowrc`; honours the
ignore lists; and reports conflicts and exit codes.

Layout: `stow/` is the engine (a deep module: `Apply` plus `Stow`/`Unstow`/`Restow`
sugar), `internal/stowpath` holds the ports of stow's path helpers that both the engine
and the CLI need (it exists because `parent` was copied instead of shared, and the copy
was wrong), `internal/getopt` is a `Getopt::Long`-compatible parser (its `Option.Validate`
is Getopt::Long's callback, which is what makes a bad `--ignore` regex a parse failure), `internal/cli` is the
front end, `internal/ui` is colour-on-a-TTY, `internal/conformance` is the differential
harness.

Validation: `go test ./...` is hermetic — including the **L5 goldens layer**, which replays
the differential fixtures against recordings of the oracle. `go test -tags oracle ./...`
additionally runs argv vectors against real `Getopt::Long`, ignore verdicts against
`Stow.pm`'s own `ignore()`, `parent` and `join_paths` against `Stow::Util`, errno strings
against Perl's `$!`, and the differential fixtures against real stow 2.4.1, comparing stdout,
stderr, exit code and the resulting tree. Install the oracle with
`PREFIX=$PWD/.oracle bash test/install-stow-oracle.sh` (`.oracle/` is gitignored; CI installs
to `/usr/local` via sudo). Regenerate goldens with
`go test -tags oracle ./internal/conformance/ -update-goldens`.

**Counts are printed by the tests themselves — never hand-copy one into a document.** Two
documents had already drifted by the time anyone checked.

The ledger is fully ruled, PL-01..PL-19. The highest-severity finding is **PL-18**:
`Stow.pm` interpolates `$ENV{HOME}` into a regex unescaped, so any user whose home path
contains `(`, `[`, `+`, `*` … cannot run stow at all. It dies before doing any work, at
every verbosity. gostow never builds that regex. Owed upstream.

Colour (SPEC §8.4) is implemented as a **rendering pass over finished lines** in
`internal/ui`, wired in once at `cli.Run`. Two invariants hold it to the parity mandate:
`StripANSI(paint(s)) == s` over every line shape, and — when colour is off — the pass does
not run at all. The engine never learns that terminals exist.

The **public API review is done** (SPEC §3.3). `Task.Source` was split into `Source` (a
link's destination) and `Dest` (a move's destination), matching `Stow.pm`'s own field
comments; the conflict banner's gerund moved out of `Action.String()` and into the CLI,
so an enum's spelling is no longer load-bearing for parity-pinned bytes.

CI runs the hermetic suite on `ubuntu-latest`, `ubuntu-24.04-arm` and `macos-latest`, so
every platform GoReleaser ships is exercised on both axes. All three pass. The oracle suite
stays linux-only: it builds GNU Stow from source, and the referent is the Perl program's
behaviour, not the platform's.

The **licence question is settled.** gostow stays MIT. It shares no source with GNU Stow;
the one substantial piece of copied *prose* — the `--help` block — was rewritten in
gostow's own words (SPEC §4.5). The ignore patterns and the error messages stay verbatim
because they *are* the behaviour, and `NOTICE` records that.

### The 2026-07-10 audit

An external audit (`docs/audit-2026-07-10.md`) found **six parity bugs**, all reproduced
against the real binary before anything was changed, all now fixed and pinned. They shared
one address: **error paths, and non-zero verbosity on a non-happy path** — regions the suite
*executed* and never *examined*. The engine's core matched the oracle under a dozen
adversarial probes.

Three lessons, all now enforced rather than remembered:

- **Coverage of a code path is not coverage of its inputs.** `parent` ran on every test run,
  never once with a single-segment absolute path.
- **`go test -coverprofile` cannot see a binary the harness execs.** `doMv` read 0% while
  fixtures drove it, because coverage is linked into the test binary, not the subprocess.
  Same blind spot as the test cache. The engine's destructive paths are now driven in
  process too. TEST-PLAN §3.6.
- **A fixture can be vacuous.** `Node.Mode` used zero to mean "unset", so `Mode: 0o000` made
  an "unreadable" directory readable. Mutate the code; watch the test go red. Every fix in
  that audit's wake was confirmed that way.

The goldens layer (L5) now exists: `go test ./...` checks gostow against recordings of the
pinned oracle, with no Perl. Regenerate with `-update-goldens`, which is declared only under
the `oracle` build tag so the hermetic suite cannot rewrite its own answers.

A seventh bug surfaced afterwards, closing the audit's one "probably fine": `Stow.pm` moves
files with `File::Copy::move`, not `rename`, *because the stow directory may be on a different
filesystem from the target* — its own comment says so. gostow used `os.Rename` and aborted
with `EXDEV`. **Ask what a fixture format cannot say**: the differential harness has one
sandbox root, so no fixture could ever put the two directories on different filesystems.
TEST-PLAN §3.5.

Still owed before v1:

- Upstream bug reports: PL-01, PL-03, PL-04, PL-05, PL-06, PL-08, PL-09, PL-10, PL-18, PL-21.
- `--adopt` across a filesystem boundary (EXDEV) has no fixture; it needs two mount points.

`chkstow` is ruled **out of scope for v1** (SPEC §12). `--compat` and the PL-04
protection asymmetry are both pinned by differential fixtures. `ignore.t` is settled without
porting it: `Stow.pm`'s own `ignore()` is the oracle.

**Do not tag v1 without a human decision.** The pre-1.0 guards stay standing until then.
