# gostow — Conformance Test Plan

Companion to `SPEC.md`. That document says *what* gostow must do; this one says *how we
prove it*, and in *what order we build it*.

The governing fact: **real GNU Stow 2.4.1 is an executable oracle.** gostow is one of the
rare projects whose specification can be *run*. The test strategy is built around that.

---

## 1. Seams under test

Per the TDD discipline, tests live at pre-agreed seams and nowhere else. **These are the
seams; they need confirmation before the first engine test is written.**

| # | Seam | What a test observes | Mirrors stow's |
|---|---|---|---|
| **S1** | `stow.Apply` (and the `Stow`/`Unstow`/`Restow` sugar) | returned `Tasks`, `Conflicts`, `error`; and the resulting filesystem tree | `stow.t`, `unstow.t`, `dotfiles.t`, `defer.t`, `cleanup_invalid_links.t` |
| **S2** | the `gostow` binary | stdout bytes, stderr bytes, exit code, resulting filesystem tree | `cli.t`, `cli_options.t`, `rc_options.t` |
| **S3** | internal pure helpers: `joinPaths`, `parent`, `adjustDotfile`, `unadjustDotfile`, the ignore matcher, `findStowedPath`, `linkDestWithinStowDir`, `foldable` | return values | `join_paths.t`, `parent.t`, `ignore.t`, `find_stowed_path.t`, `link_dest_within_stow_dir.t`, `foldable.t` |

S3 is an *internal* seam — private to the engine's implementation, exercised by the
engine's own tests. It is not part of the public interface. It earns its place because
stow's own suite tests exactly these functions, which is strong evidence they are the
right decomposition and that their edge cases are where the bugs live.

**There is deliberately no filesystem seam.** `Simulate` is planning-without-executing, not
a fake filesystem — that is how stow implements it, and symlink/folding semantics *are*
filesystem semantics. Tests run against real temp directories. A `fs` interface would be a
hypothetical seam with exactly one adapter.

---

## 2. The five layers

| Layer | Tag | Needs Perl? | What it proves |
|---|---|---|---|
| **L1** unit | — | no | S3 helpers behave (edge cases, regex compilation) |
| **L2** engine | — | no | S1: given a fixture tree, `Apply` produces the right tree/tasks/conflicts |
| **L3** CLI | — | no | S2: byte-exact streams and exit codes |
| **L4** differential oracle | `oracle` | **yes** | gostow ≡ real stow 2.4.1, on the same fixture |
| **L5** goldens | — | no | S2 again, hermetically: gostow vs what the oracle did, *recorded by L4*, frozen in `internal/conformance/testdata/goldens/` |

`go test ./...` runs L1, L2, L3, L5 — hermetic, fast, no Perl. `go test -tags oracle ./...`
additionally runs L4 against the pinned oracle.

This is the answer to the two failure modes of each approach on its own:

- **goldens alone** are never re-verified, so a transcription error becomes permanent truth.
- **a live oracle alone** forces Perl + a pinned stow onto every contributor and every CI job.

Together: goldens make the everyday suite hermetic; L4 is what keeps the goldens honest, and
it runs on every PR in CI (`ci.yml`'s `conformance` job).

Regenerate the goldens with:

```
PATH=$PWD/.oracle/bin:$PATH go test -count=1 -tags oracle ./internal/conformance/ -update-goldens
```

`-update-goldens` is declared under the `oracle` build tag, so the hermetic suite cannot
rewrite the answers it is graded against — the flag does not exist there. `OraclePath`
asserts the binary reports exactly 2.4.1 before anything is written, so a golden can only be
a recording of the pinned referent. `TestEveryCaseHasAGoldenAndViceVersa` fails if a fixture
gains no golden or a golden outlives its fixture.

The three exemptions from byte comparison — gostow's own help block, a Perl-generated
diagnostic, an errno-derived exit status — live once, in `AssertMatches`, and both L4 and L5
call it. Each exemption pins something else in exchange; none of them compares nothing.

**A `Pre` case's starting state is built by gostow in L5**, because there is no oracle to
build it with and `Snapshot` hashes file content rather than storing it. A gostow whose stow
is wrong could therefore hide a bug in a measured unstow. L4 builds that state with real stow
and would catch exactly that, on every PR.

### 2.1 Pinning the oracle

`apt install stow` on Ubuntu 24.04 gives **2.3.1**, not 2.4.1 — installing it would silently
swap the spec out from under the suite. `test/install-stow-oracle.sh` therefore builds the
checksum-pinned tarball from source and *asserts the resulting version*, failing loudly on
mismatch. Both CI and contributors use that one script.

---

## 3. The differential harness

```go
// A fixture is a declarative tree, materialised into a temp dir.
type Tree map[string]Node // path -> file{content} | dir | symlink{target}

// Snapshot serialises a directory deterministically: sorted paths, each with its
// type, symlink target (verbatim, unresolved), and content hash for regular files.
// Symlink targets are compared as STRINGS, never resolved — stow's whole contract
// is which relative link it wrote, not where it happens to point.
func Snapshot(root string) string

type Run struct {
    Stdout, Stderr string
    ExitCode       int
    Tree           string // Snapshot after the run
}

func RunBinary(bin string, args []string, env []string, cwd string) Run
```

A conformance case is:

```go
type Case struct {
    Name    string
    Stow    Tree     // contents of the stow dir
    Target  Tree     // pre-existing contents of the target dir
    Args    []string
    Env     []string // HOME, STOW_DIR, ...
    Rc      string   // optional .stowrc contents
}
```

**L4 (`//go:build oracle`)** materialises the fixture *twice*, runs real `stow` on one copy
and `gostow` on the other, and asserts all four fields of `Run` are identical. Any
difference is a conformance bug — in gostow, or a new Parity Ledger entry.

**L5** runs the same fixture against `gostow` only and diffs against `testdata/<name>.golden`.
Goldens are produced by the *oracle*, never by gostow:

```
go test -tags oracle ./... -update-goldens
```

That flag refuses to run unless `stow --version` reports exactly `2.4.1`. A golden is only
ever a recording of what real stow did.

### 3.1 What must be normalised, and what must not

Absolute paths leak into stow's output at verbosity ≥3 (`(cwd=/tmp/…)`). The harness
normalises the temp-dir prefix in `Stdout`/`Stderr` before comparison, and **nothing else**.
In particular: no whitespace trimming, no line sorting, no case folding. `RMDIR bin` must
compare unequal to `RMDIR: bin` (ledger PL-05), or the suite is not testing parity.

Per SPEC §8.2, byte-exactness is a contract at verbosity **0–2**. Levels 3–5 are compared
semantically (task sequence), not byte-wise — pending the PL-11 ruling.

### 3.2 The oracle is invisible to `go test`'s cache

**Audited 2026-07-09.** `go test` may replay a stored PASS instead of running anything. It
decides the stored result is still valid by hashing the test binary, its arguments, the
environment variables the test read, and the files it opened or stat'd **through package
`os`**. A subprocess's inputs are not in that set.

Every oracle in this repository is a subprocess. So every one of them was invisible:

| Sabotage | Package | Result before the fix |
|---|---|---|
| `Stow.pm`'s `ignore()` → `return 0` | `stow` | `ok (cached)` — 1216 verdicts against an oracle answering *no* to everything |
| `testdata/ignore_oracle.pl` → always `1` | `stow` | `ok (cached)` |
| `testdata/getopt_oracle.pl` → lies on every vector | `internal/getopt` | `ok (cached)` |
| `internal/cli`'s usage path | `internal/conformance` | `ok (cached)` — the package had no import edge to the code it tests |

Only the `stow` *binary* escaped, and only by luck: `exec.Command` stats it via `LookPath`,
and a stat entry does enter the cache key.

Two defences, both kept:

1. **`conformance.TrackOracleInput(t, paths...)`** reads each subprocess input, putting an
   `open` entry with a content hash into the test log. This makes caching *correct*.
   `conformance.PerlModulePath` asks perl where it really loaded a module from, because the
   module that matters is the one perl resolves, not the one we expected.
2. **CI passes `-count=1`.** A result that gates a release should be produced, not recalled.

The import edge for gostow's own source lives in `cli_oracle_test.go`, not in a non-test
file: package `stow`'s oracle tests import `conformance`, and `internal/cli` imports `stow`,
so a non-test edge would be an import cycle. A package's test files are not compiled into it
when another package imports it.

### 3.3 A conformance test that skips is a vacuous pass

`OraclePath`, `RequirePerl` and `OraclePerlLib` all `t.Fatal`. `OraclePath` used to `t.Skip`
when it found no stow; on a machine without one, `go test -tags oracle ./...` then printed
`ok` for every package in 0.26s rather than 5.8s, comparing nothing. The build tag *is* the
caller asking for the oracle.

The same rule pushed one skip out of the hermetic suite. `TestUnreadableIgnoreFileIsFatal`
used `chmod 000`, which does nothing to root, so under `sudo go test` it skipped. Replacing
the file with a **directory** makes it unreadable to everyone and reaches the same code path
— and doing so immediately exposed a real gostow bug (SPEC §10, PL-10): `os.Open` succeeds
on a directory, and the reader was swallowing the `EISDIR` from the first `Read`.

### 3.4 Coverage of a code path is not coverage of its inputs

**Audited 2026-07-10.** An external audit found six parity bugs. Every one of them lived in a
region this suite had *executed* and never *examined*: error paths, and non-zero verbosity on
a non-happy path. A dozen adversarial probes of the engine's core — pathological package
names, multiple stow directories, `--adopt` conflicts in both directions, `--compat` with
`--dotfiles`, stale-link restow — all matched the oracle exactly.

The shape of the gap, twice over:

- **Inputs, not lines.** `Stow::Util::parent` was executed by every test run, and never once
  with a single-segment absolute path. The value decided where the symlink farm was aimed.
- **Verbosity, not behaviour.** Every conflict fixture ran at verbosity 0, so `Stow.pm`'s
  level-2 `CONFLICT when stowing …` line had no fixture at all. The verbosity subsequence
  test cannot catch this: it compares gostow to *itself*, and a line missing at every level
  is a subsequence of a line missing at every level.

So a fixture must reach the errno-bearing paths, and it must reach them at `-vv`:

| Fixture | What it pins |
|---|---|
| a conflict at `-vv`, stowing and unstowing | the level-2 `CONFLICT` line |
| an unreadable package directory | `$!`'s wording, `(Permission denied)` and not Go's error |
| a target without its search bit, under `-n` | `canon_path`'s fatal chdir, which `-n` does not excuse |
| a `.stowrc` that is a directory | a swallowed `EISDIR`, and a package stowed that should not have been |

`Case.Root` lays a tree at the sandbox root, which is the only way to write a `.stowrc` that
is not a file. `Node.Mode` is a `*os.FileMode` for the same reason `Mode: 0o000` was a bug:
zero is a real mode, and when it doubled as "unset" the fixture asking for an unreadable
directory silently got a readable one. `RestorePermissions` runs before every snapshot and
every teardown, because a directory at mode 000 is one neither `Snapshot` nor `os.RemoveAll`
can enter.

Each of the four fixtures above was confirmed to fail when its fix is reverted. Write the
fixture, break the code, watch it go red — nothing else in this repository has earned the
right to be trusted on inspection alone.

### 3.5 `go test -coverprofile` cannot see the binary the harness execs

The audit reported `doMv` at 0%, and read that as "the hermetic suite never executes
`--adopt`'s move". Half right. L3 and L5 drive gostow as a **subprocess**, and the coverage
instrumentation is linked into the test binary, not into the one it runs. Every statement
reached only through `RunBinary` reads 0%, whether or not a fixture covers it.

This is the same blind spot as §3.2's: the tooling sees the process it is in, and the oracle,
the shell and gostow-under-test are all somewhere else. A coverage number here is a lower
bound on what is tested and says nothing about what is *not*.

So the engine's destructive and rarely-taken paths — the `--adopt` move, `isRealDir`,
`canon_path`'s fatal chdir, and the `Stow`/`Unstow`/`Restow` sugar a library consumer calls
first — are exercised **in process** in `stow/engine_test.go`, in addition to whatever
fixtures drive them through the binary. Coverage of those is now real coverage.

---

## 4. Porting stow's own suite

stow's `t/*.t` is ~478 assertions across 16 files. It is the only artefact that encodes
*intent* rather than behaviour, so it catches things a differential test cannot: a case
where gostow and stow agree because gostow copied a misreading.

Port priority, highest value first:

1. ~~`ignore.t`~~ — **done, but not by porting.** Its 287 assertions all call `Stow.pm`'s
   `ignore()` predicate directly. Rather than transcribe the expectations — which would
   re-import whatever misreading the transcriber brought — `stow/ignore_oracle_test.go`
   drives that same predicate over a matrix of ignore-file fixtures × paths, covering
   ignore.t's own cases plus the built-in list's prefix/suffix edges. The test prints the
   count it actually ran; an earlier version of this line said 1140 and was wrong by 76.
   Verified non-vacuous by mutation.
2. `stow.t` (22) and `unstow.t` (35) — the core algorithm.
3. `rc_options.t` (35) — `.stowrc` merge/precedence, where the manual is wrong (PL-01).
4. `dotfiles.t` (14) — note the tarball also ships `dotfiles.t.rej` and `unstow.t.orig`,
   evidence of an unclean 2.4.1 release; read the `.rej` files, they mark contested behaviour.
5. `join_paths.t` (22), `parent.t` (5), `find_stowed_path.t` (10),
   `link_dest_within_stow_dir.t` (6), `foldable.t` (4), `cleanup_invalid_links.t` (4),
   `defer.t` (4) — direct S3 unit tests.
6. `cli.t` (3), `cli_options.t` (10) — thin; the differential harness covers more.
7. `examples.t` (10) — the manual's worked examples. Good end-to-end sanity.
8. ~~`chkstow.t`~~ — `chkstow` is ruled out of scope for v1 (SPEC §12).

Ports are Go tests, not Perl. They are a *source of cases*, not a suite to run.

---

## 5. Build order — vertical slices

One seam, one failing test, one minimal implementation, repeat. **Not** all tests first:
bulk tests verify imagined behaviour, and here the imagined behaviour is exactly what the
oracle keeps disproving. Each slice is a tracer bullet whose result informs the next.

| # | Slice | Seam | Status |
|---|---|---|---|
| 0 | `--version` reports gostow's version | S2 | ✅ **done** — the release smoke depends on it |
| 1 | `joinPaths`, `parent` | S3 | ✅ **done** — ported from `join_paths.t`, `parent.t` |
| 2 | `--help` byte-exact, exit 0; unknown option → usage on stdout, exit 1 | S2 | ✅ **done** |
| 3 | getopt-long parser: bundling, permute, `no_ignore_case`, auto-abbrev, exact-match-wins, `-v:+` | S2 | ✅ **done** — `internal/getopt`, differentially tested against real Getopt::Long |
| 4 | stow one file into an empty target (one `LINK`) | S1 | ✅ **done** |
| 5 | tree folding | S1 | ✅ **done** |
| 6 | conflict: existing plain file → message + exit 1, nothing written | S1+S2 | ✅ **done** — proves two-phase abort |
| 7 | unstow | S1 | ✅ **done** |
| 8 | unfold (split open) and refold | S1 | ✅ **done** |
| 9 | ignore matcher + built-in defaults + the three exclusive sources | S3+S1 | ✅ **done** — `ignore.t` port still owed |
| 10 | `--dotfiles` translation | S1 | ✅ **done** — incl. the `dot-`, `dot-.x`, `dot--x` edge cases |
| 11 | `.stowrc` discovery, merge, expansion | S2 | ✅ **done** — incl. PL-01 (home-then-cwd) |
| 12 | `--adopt`, `--defer`, `--override` | S1 | ✅ **done** |
| 13 | `--compat` unstow | S1 | ✅ **done** — discriminating fixture found (SPEC §12) |
| 14 | `.stow` / `.nonstow` protection | S1 | ✅ **done** — incl. the PL-04 asymmetry |

Slice 3 is load-bearing and easy to underestimate: `pflag`/`cobra` cannot express stow's
option semantics (§4.1 of the spec), so `internal/getopt` is real work with its own unit
tests, and every later CLI slice depends on it.

---

## 6. Probes owed before implementation — **discharged (2026-07-09)**

All three source-derived ledger items have been probed against the real binary. See
`SPEC.md` §10 for the full rulings.

- **PL-06** — the `do_rmdir` undef-deref path is **unreachable**: `do_rmdir` runs only from
  `fold_tree` during `plan_unstow`, which precedes `plan_stow`, so `dir_task_for` is never
  populated when it runs. Dead code; gostow omits the branch.
- **PL-09** — **confirmed**. A target symlink whose destination is exactly `0` aborts the
  unstow with `stow: ERROR: Could not read link <path>`, exit 2. `00` and `0.0` are fine.
  Perl falsiness bug; ruled *do not replicate*.
- **PL-10** — **confirmed**. An existing-but-unreadable `.stow-local-ignore` silently
  disables *all* ignoring, so `README.md` is stowed and the ignore file stows itself, exit 0,
  no warning below verbosity 5. Ruled *do not replicate*; gostow makes it a fatal error.

## 7. What we do not test

- **stow's Perl internals.** We test observable behaviour at S1/S2 and pure functions at S3.
- **Verbosity ≥3 byte output**, pending PL-11.
- **Performance.** stow is not fast; being faster is free and unmeasured.
- **`--adopt` across a filesystem boundary.** `os.Rename` and Perl's `rename` both fail with
  `EXDEV`, and since 2026-07-10 the *message* is errno-derived on both sides — but no fixture
  proves it, because one needs two mount points. Named here rather than assumed away.
- **Perl-only regex constructs.** Lookaround and backreferences are rejected at parse time
  (PL-22), which is asserted; what they would have *matched* is not, and cannot be.
