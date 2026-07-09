# gostow + dstow — Concept & Spec (design capture)

**Status:** design discussion captured 2026-07-09. Nothing built yet. This is a
working concept doc, not committed project documentation.

These are **two new, separate tools**, distinct from `dotd`/dot-dagger. They live
(or will live) in their own repositories and ride the same over-engineered
release/distribution machinery this project built (`rocne/release-ci` reusable
workflow, signing, package repos). The tools themselves are small; they merely get
*distributed* like serious software.

---

## Why these exist (and why NOT a mode of dotd)

`dotd` is deliberately over-engineered as an *exercise in robust distribution* — a
DAG/predicate engine with a large conceptual surface. That is its whole reason to
exist, but it means it is a big conceptual leap and (as of this writing) not yet
usable as a daily-driver dotfiles tool for real work.

The new tools invert that thesis: **small, elegant, no new concepts, get out of the
way.** Because the theses are *opposed*, and because the engines don't overlap
(dotd reimplements symlinking on its own model; gostow faithfully clones GNU stow),
a "simplified mode of dotd" is a category error. Decision: **separate tools.**

Relationship going forward:
- **dstow** is intended to be the daily driver the user actually adopts now.
- **dotd** remains the someday-robust, teaches-the-pattern project.
- Using dstow daily is expected to *inform* dotd's UX (which is under audit).

---

## The two-tool split (engine vs. product)

### gostow — the engine

- **Thesis:** GNU stow, reimplemented in Go, shipped as a single static binary
  (no Perl). "Just stow, in Go. No ifs, ands, or buts."
- **Fidelity contract — TOTAL PARITY.** From the user's perspective there is *no
  difference* between gostow and stow: every existing script, config, flag, and
  option works identically. The **only** things we do not replicate are stow's
  bugs and accidental/unexpected behavior. Everything intentional we match.
- **The one additive liberty:** color on a TTY (real stow emits none; coloring a
  TTY cannot affect any script or pipe). Slogan: **byte-compatible on a pipe,
  prettier on a TTY.**
- **Config-less.** gostow invents no config of its own — flags only, like stow.
  It *does* faithfully honor stow's own `.stowrc` and ignore files (see below);
  that is stow's format, not gostow's config.
- **Deep module:** narrow public API (`stow` / `unstow` / `restow`), hiding the
  hard parts (tree folding, conflict detection, dot-prefix translation, ignore
  handling). Its **spec is stow itself** → ideal test-first project; conformance
  tested against real stow.
- **Independently valuable:** "GNU stow in Go, single binary, no Perl" is a real
  tool with its own audience, separate from dstow.

### dstow — the product

- Built **on top of gostow as a library.** Adds all ergonomics; owns all config.
- Concepts:
  1. **Location independence** — run `dstow <pkg>` from anywhere (vs. stow forcing
     `cd` into the stow dir). The primary ergonomic win.
  2. **Package-local config** — the per-package config (a `.stowrc`-equivalent)
     lives *inside* the package, so a package is self-describing. (Contrast: stow's
     `.stowrc` sits at the packages-root.)
  3. **Lifecycle hooks** — a `.dsto/` subdir per package with pre/post scripts
     (later: startup/cleanup), auto-ignored by the stow engine.
  4. **Multi-level hooks** — hooks definable at multiple levels (package *and*
     repository level). Solves "install prerequisites once" without hand-rolling it
     in every package. This is the near-term extensibility idea (a bare-bones
     plugin system that is really just hooks-at-levels).
  5. **`DSTO_PATH`** — a `$PATH`-style search path of package sources. Composable.
  6. **Git-tap sources** — homebrew-style `user/repo` → assume `github.com/user/repo`,
     `git clone` into the XDG-local dstow packages dir (which is on `DSTO_PATH`).
     **Keep it dumb:** just a clone into a managed dir. No registry, no lockfile,
     no dependency resolution — the moment it grows those it has re-invented dotd.
  7. **(Maybe) system requirements** — declare prerequisite OS packages (ripgrep,
     etc.). Unsure if already prototyped; low priority.

---

## Faithful vs. predictable (the folding decision)

Tree-folding (symlinking a whole directory when possible) is stow's cleverest and
*most surprising* behavior — the thing that confuses newcomers. For a
simplicity-focused tool this is a real design lever.

**Resolution — and it dissolves a boundary problem:** GNU stow already has
`--no-folding`. So "predictable / no-fold" is a *native, faithful* stow feature,
not an invention.

- **gostow** implements `--no-folding` because pure stow has it — zero compromise
  to its parity thesis. The engine library takes `fold` as a parameter; the gostow
  CLI defaults to folding-on, exactly like stow.
- **dstow's** faithful/predictable toggle is then nothing but "pass `fold=false`."
  No new engine behavior, no duplication.

**dstow policy:**
- **Default: utterly faithful** (folding on, stow behavior).
- **Loudly advertise predictable** — on setup, big clear recommendation to switch
  to predictable mode; but the default stays faithful.
- Setting lives in **dstow's** XDG config (dstow owns config; gostow stays
  config-less).

**Rejected: per-package fold toggle.** Folding is a property of a *target subtree*,
not a package — two packages can write into the same target dir, and that subtree
cannot be simultaneously folded (for A) and unfolded (for B). A per-package flag
would let you declare contradictions the engine can't satisfy. (Stow resolves this
dynamically by unfolding on demand.) A **global** setting is coherent; per-package
is ill-defined. Dropped.

---

## Comparison to the field (why dstow over dotbot etc.)

Landscape splits on **manifest-driven vs. convention-driven**:
- **GNU stow** — pure convention (dir structure *is* config); no hooks, no
  per-package config, cwd-bound, needs Perl.
- **dotbot** — manifest-driven (`install.conf.yaml` lists every link); has a global
  `shell` hook directive; distributed as a committed git submodule; Python.
- **chezmoi** — heavyweight (templating, secrets, encryption, machine state);
  single Go binary. This is dotd's spiritual sibling in ambition; not dstow's lane.
- **yadm / rcm** — git-$HOME-centric / tag-and-host-based.

dstow's differentiators (incremental over dotbot, but the increments are exactly
daily-driver ergonomics):
1. **Convention over manifest** — drop a directory in; never edit a central YAML.
2. **Package-local encapsulation** — a package carries its own config + hooks +
   (maybe) requirements, all together. Locality = discoverability.
3. **Multi-level, package-bound lifecycle hooks** — solves prerequisite install.
4. **`DSTO_PATH` composable sources** — nobody does this cleanly.
5. **Single standalone binary** — no Perl, no Python.

Honest framing: dstow competes on *feel*, not feature count — the correct thing to
optimize for a personal daily driver. It's "stow with the ergonomics stow refused
to add." (If an authoritative comparison table is ever wanted for a README, verify
against live dotbot/chezmoi docs rather than memory.)

---

## Shared output/color library — direction yes, timing NOT yet

Examined dotd's output/color code (`internal/ui/`, `internal/log/`). Findings:

- **`internal/ui/ui.go`** splits into two halves with *opposite* reusability:
  - **Semantic vocabulary** (`OK`/`Missing`/`Wrong`/`Conflict`/`Installed`/
    `Installable`/`Skip`/`HardMissing`/`Arrow`/`Key`) = **dotd's domain language**
    (predicate-state vocabulary). **Policy — not reusable, and shouldn't be.** Each
    tool has its own vocabulary (stow: LINK/UNLINK/conflict).
  - Print helpers + palette = the *shape* is generic; the specific labels are
    dotd-flavored.
- **`internal/ui/cobra.go`** — colored cobra help/usage templates (~100 lines).
  **Domain-free and cleanly extractable. The prize.**
- **`internal/log/log.go`** — ~30 lines of charmbracelet/log setup. Generic but so
  small that sharing is marginal.
- **Key reframe:** the color *disable* logic (NO_COLOR / not-a-TTY) is **not
  dotd's** — it's `fatih/color`'s built-in behavior. The hard infra is already a
  third-party lib; dotd only added a thin semantic layer. So the true reusable
  surface is small.

**Principle: separate plumbing from policy.**
- Plumbing (reusable, domain-free): cobra-color glue + a `Labelf(w, label, color,
  format, …)` mechanism. ~100–150 lines total.
- Policy (per-tool, never shared): the vocabulary — the part that *feels* like "our
  unified output" is exactly the part that must stay per-tool. Most shared-output
  libraries fail by trying to share policy.

**Recommendation: do NOT extract speculatively now.**
1. Only one real consumer (dotd). Extracting from a single witness bakes dotd's
   shape into a "shared" API you'll fight in dstow. Rule of Three: at ~100 lines,
   *copy* the cobra glue into gostow/dstow freely; extract once used 2–3 ways and
   the shape has stopped moving.
2. **gostow is the wrong validator** (same reason as the fold toggle): it must
   mimic stow, may want *almost none* of this, and real stow isn't even cobra-based.
   Only **dstow** can tell you what the house style should be. Extract when dstow
   exists and reuses it — not when starting gostow.

---

## gostow loose ends (the whole decision surface)

1. **Fidelity contract:** total parity (scripts/configs/flags/options identical);
   do not replicate bugs; color-on-TTY is the sole additive liberty.
2. **`.stowrc` + ignore files:** must read `.stowrc` and honor
   `.stow-local-ignore` / `.stow-global-ignore` + stow's built-in ignore defaults.
   (Stow's format, not gostow config.)
3. **`--dotfiles` mode:** in — dot-prefix ↔ `.` translation, 100% included.
4. **Pin a target stow version** as the conformance referent (a modern, recent,
   stable release, e.g. the 2.4.x line — confirm exact latest stable when building).
5. **Flag-parsing style:** stow uses getopt-long (`-t DIR`, `--target=DIR`, …). If
   reaching for cobra/pflag, ensure flag *semantics* match, not just names —
   parity extends to how options parse.

---

## dstow — texture to preserve for handoff

- **Reference implementation exists.** dstow derives from a roughed-out Bash
  wrapper the user already uses daily at work. When speccing dstow, **retrieve
  that actual script first** — it is the concrete source of truth for intended
  behavior, not just this doc. Much of dstow's "spec" is "make the good parts of
  that wrapper robust and installable."
- **`DSTO_PATH` vs. a paths-config.** Two ways to declare where packages live were
  floated: (a) the `$PATH`-style `DSTO_PATH` env var, and (b) a dstow config file
  listing package roots. These are not mutually exclusive (env for ad-hoc/session,
  config for persistent). The git-tap clone target is one such root on that path.
- **dstow is the intended daily driver**; its ergonomics should be validated by the
  user actually living on it, and those lessons feed back into dotd's UX audit.

## Open / deferred

- dstow package config filename + schema (a `.dsto`-dir manifest vs. a dotfile).
- Exact hook lifecycle set (pre/post now; startup/cleanup/repo-level later).
- Whether system-requirements declaration ships v1 or later.
- **Repo layout: TWO repos** (decided 2026-07-09) — separate, not a monorepo.
  gostow is independently valuable ("stow in Go" for users who never touch dstow),
  so it gets its own repo and independent release line; dstow imports gostow as a
  library. Create gostow's repo first (it's the dependency) and spec it there.
- Module names (gostow, dstow) and Go module paths still to be chosen.
- When to extract the shared plumbing lib (trigger: dstow reuses it).
