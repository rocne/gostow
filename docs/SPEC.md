# gostow — Conformance Specification

**Status:** test-first, under construction. Ledger fully ruled; engine in progress.
**Conformance referent:** GNU Stow **2.4.1** (released 2024-09-08; latest stable as of 2026-07-09).

This document *is* the spec in the sense that it records what stow 2.4.1 does. Where
this document and real stow 2.4.1 disagree, **real stow wins** and this document is
the bug. Every claim below is tagged with how it was established:

- **[probed]** — verified by executing real `stow` 2.4.1 and observing behaviour.
- **[source]** — read directly from stow 2.4.1's implementation.
- **[inferred]** — deduced from source but not yet exercised; must be probed before relying on it.

---

## 1. The conformance referent

| | |
|---|---|
| Version | 2.4.1 |
| Released | 2024-09-08 |
| Tarball | `https://ftp.gnu.org/gnu/stow/stow-2.4.1.tar.gz` |
| sha256 | `2a671e75fc207303bfe86a9a7223169c7669df0a8108ebdf1a7fe8cd2b88780b` |

### Where the documented behaviour comes from

Four sources, in decreasing authority:

1. **The installed binary and modules** — `stow` (CLI, 853 lines), `Stow.pm` (engine, 2530
   lines), `Stow/Util.pm` (267 lines). This is the ground truth and the differential-test
   oracle.
2. **stow's own test suite** — `t/*.t` in the tarball: 16 files, ~3,055 lines, ~478
   assertions. These encode *intent*, which the source alone does not. Porting them is a
   deliverable (see `TEST-PLAN.md`).
3. **The Texinfo manual** (`info stow`) and **man page** (`stow.8`) — normative prose, but
   demonstrably drifts from the code (§4.1 documents one such drift).
4. `NEWS` / `ChangeLog` — for understanding *why* a behaviour exists.

Sources 3 and 4 are commentary. When they contradict source 1, source 1 is the spec.

---

## 2. The parity model

### 2.1 What parity means

From a consumer's perspective — a script, a `.stowrc`, a pipe, a `$?` check — gostow and
GNU stow 2.4.1 are indistinguishable. The single additive liberty is **colour on a TTY**:
*byte-compatible on a pipe, prettier on a TTY.*

### 2.2 The litmus (governing rule for every discrepancy)

"Bug" is defined **narrowly**. It does not mean "output we find ugly." The moment we
normalise output we judge accidental, every byte becomes a debate and we lose real stow as
an objective oracle. Therefore:

| Tier | Character | Ruling |
|---|---|---|
| **1** | Reproducible and dependable — stdout/stderr bytes, normal exit codes | **Replicate byte-for-byte**, cosmetic warts included. |
| **2** | Non-reproducible or undefined — errno-derived exit codes, crashes, undef derefs | **Do not replicate.** Substitute stable, defined behaviour. |
| **3** | Reproducible, but a behavioural footgun on standard usage | **Default to replicate.** Record in the Parity Ledger; diverge only by explicit ruling. |

Every discrepancy discovered goes in the **Parity Ledger** (§10) with its tier and ruling.
Tier-1 and tier-2 items are settled by the litmus alone. Tier-3 items are batched for a
human ruling — they do not block progress; the default is replicate.

**v1's job is to be stow**, including its reproducible warts and loud failures. stow's
*documented* known bugs (the empty-directory problem; tree-folding across multiple stow
directories) are **replicated in v1**. Fixing them would be a different, better-than-stow
project.

Where a wart is worth fixing *upstream*, the ledger records that too — gostow's audit of
stow is a genuine artefact, and several entries deserve a bug report to `bug-stow@gnu.org`.

---

## 3. Engine public API

The engine is a **deep module**: one entry point hides tree folding, unfolding, refolding,
conflict detection, dot-prefix translation, ignore resolution, and two-phase task planning.

```go
package stow // github.com/rocne/gostow/stow

type Action int
const (
    ActionStow Action = iota
    ActionUnstow
    ActionRestow
)

// Request is one positional action group, mirroring stow's `-D pkg1 -S pkg2` form.
type Request struct {
    Action   Action
    Packages []string
}

type Options struct {
    Dir       string    // stow directory (required)
    Target    string    // target directory (required)
    Fold      bool      // true = stow's default; CLI --no-folding sets false
    Dotfiles  bool      // dot- <-> . translation
    Adopt     bool      // move conflicting plain files into the package
    Compat    bool      // legacy unstow algorithm (-p): traverse target tree
    Simulate  bool      // plan and report; never touch the filesystem
    Verbosity int       // 0..5
    Ignore    []string  // raw regex sources, in stow's precedence order
    Defer     []string
    Override  []string
    Log       io.Writer // nil => io.Discard. The CLI passes os.Stderr.
    FixQuirks bool      // fix stow's defects instead of matching them; see §8.36
}

// Apply is the deep entry point. It plans every request, detects all conflicts
// across all requests, and only then mutates the filesystem — matching stow's
// two-phase all-or-nothing semantics.
func Apply(opts Options, reqs ...Request) (*Result, error)

// Sugar over Apply, and the API named in the design brief.
func Stow(opts Options, pkgs ...string) (*Result, error)
func Unstow(opts Options, pkgs ...string) (*Result, error)
func Restow(opts Options, pkgs ...string) (*Result, error)

type Result struct {
    Tasks     []Task     // ordered, post-cancellation; empty when Simulate turned up nothing
    Conflicts []Conflict // non-empty implies Apply returned *ConflictError
}

type Task struct {
    Action TaskAction // Create, Remove, Move
    Type   TaskType   // Link, Dir, File
    Path   string     // relative to Target
    Source string     // what a symlink points at; links only
    Dest   string     // where a file moves to (--adopt); moves only
}

type Conflict struct {
    Action  Action // the action being planned when the conflict arose
    Package string
    Message string // byte-exact stow wording; see §8.3
}

type ConflictError struct{ Conflicts []Conflict }
```

### 3.1 Why this shape

- **`fold` is a parameter, not a mode.** `--no-folding` is native to stow, so dstow's
  "predictable mode" is nothing but `Fold: false`. No new engine behaviour.
- **`Apply` is the seam.** stow's conflict semantics are *all-or-nothing across the whole
  invocation*: `stow -D a -S b` plans the unstow of `a` and the stow of `b`, checks every
  conflict, and aborts everything if any exists. **[source: `stow` main()]** A per-package
  `Stow()`/`Unstow()` API cannot express that, so `Apply` is the real interface and the
  three named functions are sugar over it.
- **Ordering is part of the interface.** `Apply` plans *all* unstow requests before *any*
  stow request, regardless of argument order. **[source]** `ActionRestow` enqueues the
  package into both lists. **[source]**
- **No filesystem interface.** `Simulate` is implemented the way stow implements it — plan
  the tasks, log them, decline to execute — not by swapping in a fake filesystem. Symlink
  and folding semantics *are* filesystem semantics; tests run against real temp
  directories. Introducing an `fs` seam would be a hypothetical seam with one adapter.

### 3.2 Internal seams (private; not part of the interface)

`joinPaths`, `parent`, `adjustDotfile`, `unadjustDotfile`, the ignore matcher, and
`findStowedPath` are unit-tested directly. stow's own suite tests exactly these
(`join_paths.t`, `parent.t`, `ignore.t`, `find_stowed_path.t`,
`link_dest_within_stow_dir.t`), which is strong evidence they are the right internal seams.

### 3.3 API review before the v1 freeze (2026-07-09)

A `v1` tag freezes every exported name. Two defects were found and fixed; two suspicions
were examined and deliberately left alone.

**Fixed — `Task.Source` was two fields wearing one name.** It held a symlink's destination
for a link *and* a file's move destination for a move, so a move's **destination** lived in
a field called `Source`. That is not a shortcut, it is an error, and `Stow.pm` does not make
it: its task comment reads `source => (only for links)` / `dest => (only for moving files)`.
Split into `Source` and `Dest`, matching stow's own vocabulary. The `--adopt` differential
fixture covers the move path — swapping `os.Rename`'s arguments fails `TestEngineAgainstOracle`.

**Fixed — the conflict banner was built out of an enum's `String()`.** The CLI printed
`"WARNING! %sing %s ..."` from `Action.String()`, which made `"stow"` a load-bearing
*spelling* of `ActionStow`: renaming the Stringer would silently move parity-pinned bytes.
The gerund now lives in `internal/cli`, where the words gostow prints belong, and
`Action.String()` documents that it is for diagnostics only.

**Left alone — `Task.Action` and `Conflict.Action` are different types sharing a name.**
One is create/remove/move, the other stow/unstow/restow. `Stow.pm` calls both `action`, Go's
type system makes confusing them a compile error, and renaming would trade stow's vocabulary
for a problem the compiler already solves.

**Left alone — `Conflict.Message` is a preformatted string.** A structured conflict would
serve dstow better, but the message text is dictated by parity and the engine is the only
thing that knows enough to produce it. Revisit when dstow has a concrete need; adding a
structured field later is backwards-compatible, changing `Message`'s meaning is not.

---

## 4. CLI surface

### 4.1 Option parsing semantics

stow calls `Getopt::Long::config('no_ignore_case', 'bundling', 'permute')`. **[source]**
Parity extends to *how* options parse, not merely their names. **These semantics rule out
`pflag`/`cobra` flag parsing**, which cannot express them; gostow needs a `Getopt::Long`-
compatible parser in `internal/getopt`.

| Behaviour | Effect | Evidence |
|---|---|---|
| `bundling` | `-nv` == `-n -v`; `-ttgt` == `-t tgt` | **[probed]** |
| `no_ignore_case` | `-v` (verbose) and `-V` (version) are distinct | **[probed]** |
| `permute` | `stow pkg -n -v` == `stow -n -v pkg` | **[probed]** |
| `auto_abbrev` (on by default; **not** disabled) | `--targ=tgt` == `--target=tgt`; `--dot` == `--dotfiles`; `--i=f` == `--ignore=f` | **[probed]** |
| exact match beats abbreviation | `--no` is `--simulate` (exact alias), **not** an ambiguous prefix of `--no-folding` | **[probed]** |
| ambiguous abbreviation | `--ver` → `Option ver is ambiguous (verbose, version)` on stderr, usage on stdout, **exit 1** | **[probed]** |
| `--` terminator | Getopt stops; remaining args are **discarded, not treated as packages**. `stow -- pkg` → `stow: No packages to stow or unstow`, exit 1 | **[probed]** — ledger PL-03 |
| abbreviation is canonicalised in diagnostics | `--tar` (missing value) → `Option target requires an argument`; but the exact alias `--d` → `Option d requires an argument` | **[probed]** |
| a value-taking option eats the rest of its bundle | `-Dpkg` is `-D` then the bundle `pkg`: `p` is `--compat`, then `Unknown option: k`, `Unknown option: g` | **[probed]** |
| `-v` bundle remainder is a value only if it is an integer | `-v3` sets 3; `-vv` increments twice; `-v=3` increments, then `Unknown option: =`, `Unknown option: 3` | **[probed]** |
| `:+` swallows a numeric next argument | `-v 3` and `--verbose -1` set 3 and −1; `--verbose pkg` increments and leaves `pkg` a package | **[probed]** |
| empty value: `=s` vs `:+` | `--dir=` → `Option dir requires an argument`; `--verbose=` → *increments* | **[probed]** |
| errors do not stop parsing | Getopt::Long accumulates diagnostics; stow prints them all, then usage | **[probed]** |
| the invalid-integer wording is **not stow's** | `(integer number expected)` since Getopt::Long 2.55; `(number expected)` before it | **[probed]** — ledger PL-19 |

`internal/getopt` is validated by driving real `Getopt::Long`, configured exactly
as stow configures it, over 6307 generated argv vectors and comparing the resulting
options hash, both package lists, the leftover array and every diagnostic
(`go test -tags oracle ./internal/getopt/`).

### 4.2 Options

| Option | Spec | Notes |
|---|---|---|
| `-d DIR`, `--dir=DIR` | `dir\|d=s` | Also sets default target to `parent(DIR)` |
| `-t DIR`, `--target=DIR` | `target\|t=s` | |
| `-S`, `--stow` | positional action switch | Default action |
| `-D`, `--delete` | positional action switch | |
| `-R`, `--restow` | positional action switch | Enqueues package into *both* lists |
| `--ignore=REGEX` | repeatable | Compiled as `(REGEX)\z` — **suffix match** |
| `--defer=REGEX` | repeatable | Compiled as `\A(REGEX)` — **prefix match** |
| `--override=REGEX` | repeatable | Compiled as `\A(REGEX)` — **prefix match** |
| `--adopt` | | Moves conflicting plain files into the package |
| `--dotfiles` | | §6 |
| `--no-folding` | **not listed in `--help`** | Real flag; documented in the man page only |
| `-p`, `--compat` | | Legacy unstow: traverse target tree, not package tree |
| `-n`, `--no`, `--simulate` | | |
| `-v`, `--verbose[=N]` | `verbose\|v:+` | `-v` *increments*; `--verbose=N` *sets*. `-vv` == `--verbose=2`; `-vv --verbose=0` == level 0 **[probed]** |
| `-V`, `--version` | | stdout, exit 0 |
| `-h`, `--help` | | stdout, exit 0 |

`Stow.pm` additionally accepts `conflicts`, `paranoid`, and `test_mode` options with **no
CLI flag**. **[source]** gostow's engine omits them; they are not reachable behaviour.

### 4.3 Package name handling

- Trailing slashes are stripped (`stow pkg/` works — shell tab-completion ergonomics). **[probed]**
- Any remaining `/` → fatal: `stow: ERROR: Slashes are not permitted in package names`. **[probed]**
- A package that is not a directory under the stow dir → fatal:
  `stow: ERROR: The stow directory <stow_path> does not contain package <pkg>`, where
  `<stow_path>` is expressed **relative to the target**. **[probed]**

### 4.4 Program name and `--version` — a **ruled** parity exception

stow's version and usage banners use `basename($0)`, printing
`stow (GNU Stow) version 2.4.1`. **[source]**

**gostow's `--version` reports gostow's own version.** This is a deliberate, ruled break
with byte parity:

```
gostow 0.1.0 (GNU Stow 2.4.1 compatible)
```

**Rationale.** Parity is owed to the *behaviour scripts depend on* — the symlink farm, the
conflict semantics, the exit codes, the operation log. It is not owed to the tool's
self-identification. When you run a tool and ask its version, you want *that tool's*
version; which upstream it mimics is an implementation detail, worth stating alongside but
never instead. A `--version` that reports `2.4.1` for a binary released as `0.1.0` is not
fidelity, it is theatre — and it makes the release pipeline's install smoke test
(`gostow --version | grep -F "$VER"`) unsatisfiable.

The same identity line opens the `--help` banner. Everything else in the help block — the
synopsis, the option list, its wording and spacing — remains byte-exact (§8.4), including
the omission of `--no-folding` (PL-16).

Beyond the identity line, `--help` gains two additive lines naming gostow's own flags
(§8.35). Nothing stow prints is altered, reordered or removed.

### 4.4.1 Program name is `basename($0)` — replicate the mechanism

`$ProgramName` is not the constant `stow`; it is `basename($0)`. Symlink stow to `mystow`
and it answers `mystow: No packages to stow or unstow`, prints `mystow [OPTION ...]` as its
synopsis, and banners `mystow (GNU Stow) version 2.4.1`. **[probed]** Ledger PL-17.

gostow derives the program name from `os.Args[0]` the same way, so every usage error, fatal
error and synopsis line is byte-exact when gostow is installed under the name `stow` — the
drop-in case. The **identity line alone** ignores `$0` and says `gostow`, because it names
the tool rather than the invocation.

This is also what makes the differential harness strict: it builds gostow's binary *named
`stow`*, so stderr compares byte-for-byte, and only the identity line is expected to
differ. Note `Unknown option: foo` carries **no** program-name prefix at all —
Getopt::Long emits it directly.

### 4.5 Help **prose** is not part of parity — a **ruled** exception

gostow's `--help` was originally GNU Stow's block, transcribed byte for byte. It is now
gostow's own prose. **Ruled 2026-07-09** on two independent grounds, either of which
suffices.

**Licensing.** GNU Stow is GPLv3-or-later; gostow is MIT. Option *names*, their semantics,
their parsing and their observable behaviour are interface facts that anyone may
reimplement — that is what the whole engine does, and it is not in question. But 34 lines
of English prose (1644 bytes, measured) are stow's *expression*. Redistributing them under
MIT offers rights we do not hold.

**Correctness.** The transcribed block ended:

```
Report bugs to: bug-stow@gnu.org
```

so gostow directed *its own* bug reports to the GNU Stow mailing list. Faithfully
reproducing that is not fidelity; it is a defect, and a discourteous one.

#### What the mandate actually promises

> every existing script, config, flag, and option behaves identically

Help prose is not a script, a config, a flag, or an option. **Option parsing is**, and it
remains byte-exact — pinned by 6307 argv vectors against real `Getopt::Long` (§4.1). No
command line GNU Stow accepts is parsed differently by gostow, and no output a script reads
has changed. The usage *diagnostic* on stderr and the exit code stay byte-exact too; only
the help block dumped alongside them on stdout is gostow's.

#### What is checked instead

Four properties replace the byte comparison, and each is stronger, in the dimension that
matters, than "the prose is identical":

| Property | Where | Layer |
|---|---|---|
| Every option **GNU Stow documents**, gostow documents. | `TestHelpDocumentsEveryOptionStowDocuments` | differential |
| Every option **gostow's parser accepts**, gostow documents. | `TestHelpDocumentsEveryOption` | hermetic |
| A usage error prints exactly that binary's own `--help` on stdout. | `Case.UsageOnStdout` | differential |
| `--help` names gostow's bug tracker, never stow's. | `TestHelpPointsBugsAtGostow` | hermetic |

The converse of the first is deliberately **not** asserted: gostow's help may name *more*.
It names its own `--gostow-*` extensions, and it names **`--no-folding`** — a real,
working flag that stow's help has never mentioned (PL-16). The second property is exactly
the check GNU Stow lacks.

Freed from transcription, the block is grouped, wrapped, and points at GNU Stow's manual as
the authority on what the shared options mean. There is no second description to keep in
sync.

---

## 5. Option resolution: `.stowrc`, `STOW_DIR`, defaults

Resolution order, in the order the code performs it **[source]**:

1. Parse the command line.
2. Parse `.stowrc` files and parse the concatenation with *the same parser*.
3. Merge: for **list-valued** options (`ignore`, `defer`, `override`), rc options come
   **first**, then CLI options, appended. For **scalar** options, the CLI **overwrites**
   the rc value. **[probed]**
4. Resolve `dir`: if still unset, use `$STOW_DIR` if non-empty, else `getcwd()`. Because
   this happens *after* the merge, **`--dir` in `.stowrc` beats `$STOW_DIR`**. **[probed]**
5. Validate `dir` is a directory; else usage error, exit 1. **[probed]**
6. Resolve `target`: if unset, `parent(dir)`, falling back to `.`. Validate if given.

### 5.1 `.stowrc` file discovery — the manual is wrong

The man page states stow reads *"`.stowrc` (current directory) and `~/.stowrc` (home
directory) in that order."* **The code does the opposite**: it builds `('.stowrc')` then
`unshift`es `"$HOME/.stowrc"`, yielding `[~/.stowrc, ./.stowrc]`. **[source]** Both files
are read and their tokens **concatenated**, then parsed as one option array — so for scalar
options **last wins**, i.e. **`./.stowrc` overrides `~/.stowrc`**. **[probed]**

gostow follows the **code**, not the manual. Ledger PL-01.

### 5.2 `.stowrc` syntax

- Each line is split with Perl's `shellwords` (POSIX-ish shell word splitting; honours
  quotes and backslash escapes). **[source]**
- **There is no comment syntax.** `#` is not special to `shellwords`. A line
  `--ignore=keep # a comment` parses as three tokens: `--ignore=keep`, `#`, `comment`. The
  latter two are read as *package names*, and rc package names are discarded — so comments
  appear to work **by accident**. **[probed]** Ledger PL-02.
- `-D`, `-R`, `-S` and any package names in an rc file are parsed and **ignored**. **[probed]**
- `--target` and `--dir` undergo environment-variable and tilde expansion:
  - `$VAR` and `${VAR}`, unless preceded by a backslash; `\$` unescapes to `$`.
  - A reference to an **undefined** variable is fatal:
    `--target option references undefined environment variable $FOO; aborting!` **[probed]**
  - `~` and `~user` expand to home directories; `\~` unescapes.

---

## 6. `--dotfiles` translation

Translation is `s/^dot-([^.])/.$1/`, applied **per path segment**, to the *package-side*
name to produce the *target-side* name. **[source]**

| Package node | Target node | Why |
|---|---|---|
| `dot-bashrc` | `.bashrc` | ordinary case |
| `dot-config` | `.config` | |
| `dot--dash` | `.-dash` | `-` satisfies `[^.]` |
| `dot-` | `dot-` | nothing follows the prefix |
| `dot-.hidden` | `dot-.hidden` | next char is `.` |
| `notdot-x` | `notdot-x` | prefix is anchored |

All six rows **[probed]**. Nested segments translate independently:
`dot-config/dot-nvim/init.lua` → `.config/.nvim/init.lua` (visible only under
`--no-folding`; otherwise `.config` folds to a single symlink). **[probed]**

The inverse (`unadjust_dotfile`: `s/^\./dot-/`, with `.` and `..` exempt) is used **only**
when unstowing with `--compat --dotfiles`, because compat mode walks the target tree.
**[source]**

**Ignore matching happens before dot-translation** — `ignore()` is called on the untranslated
`$target_node_path`, then the node is adjusted. **[source]** This is subtle and is exactly
the bug 2.4.1's NEWS claims to have fixed; it must be probed carefully.

---

## 7. Algorithm

### 7.1 Two-phase execution

Plan every requested action into an ordered task list, recording conflicts rather than
failing fast. If **any** conflict exists, print them all and abort **without touching the
filesystem**. Otherwise execute the tasks in order. **[source]**

Task planning cancels redundant work: creating a link that is already scheduled for
removal with the same destination *reverts* the removal (both become no-ops); duplicate
creates collapse. Cancelled tasks are stripped before execution. **[source]**

### 7.2 Stowing

For each node in the package (sorted by name, `.`/`..` skipped) **[source]**:

- If the node is an **absolute symlink** in the package → conflict
  (`source is an absolute symlink ... => ...`). Stow only ever creates relative symlinks. **[probed]**
- If the target path **is a symlink**:
  - not owned by stow → conflict `existing target is not owned by stow: <path>`
  - points into a stow package that exists:
    - identical destination → skip (`--- Skipping ... as it already points to ...`)
    - matches `--defer` → skip (`--- Deferring installation of: ...`)
    - matches `--override` → unlink, relink **[probed]**
    - both old and new destinations are directories → **unfold**: unlink, mkdir, restow the
      *existing* package's contents there, then the new package's **[probed]**
    - otherwise → conflict `existing target is stowed to a different package: <p> => <dest>`
  - points into a stow package that does **not** exist (dangling) → replace with a good link
- If the target path **is a directory** → recurse into it (or conflict if the package node
  is not a directory)
- If the target path exists and is neither → `--adopt` moves it into the package and links;
  otherwise conflict
  (`cannot stow <src> over existing target <t> since neither a link nor a directory and --adopt not specified`)
- If `--no-folding` and the package node is a real directory → `mkdir` + recurse
- Otherwise → **fold**: create one symlink for the whole subtree

### 7.3 Unstowing

Traverses the **package** tree (or, with `--compat`, the **target** tree). **[source]**
Removes symlinks that point into the package being unstowed; leaves everything else alone,
including unowned links that would have conflicted on stow. Absolute symlinks in the target
are skipped with `Ignoring an absolute symlink: <t> => <d>` on stderr.

After unstowing a directory's contents, `cleanup_invalid_links` removes links that are both
**orphaned** and **owned by stow** (they point at a non-existent path inside a stow
package), because they would otherwise block refolding. **[source]**

### 7.4 Folding, unfolding, refolding

- **Fold** — when a whole package subtree can be represented by one symlink, it is. **[probed]**
- **Unfold ("split open")** — when a second package needs a folded directory, the symlink is
  removed, a real directory created, and *both* packages' contents linked into it. Task
  order: `UNLINK`, `MKDIR`, existing package's nodes, then new package's nodes. **[probed]**
- **Refold** — on unstow, a directory containing only links into a single surviving package
  is collapsed back to one symlink. Task order: `UNLINK` each node, `RMDIR`, `LINK`. **[probed]**
- `--no-folding` disables folding on stow **and** refolding on unstow. **[source]**

A directory is foldable iff every node in it is a link, all links share the same parent
directory inside the package, and that parent is owned by stow. **[source]**

### 7.5 Protected directories

`should_skip_target` refuses to descend into: the current stow directory; any directory
containing a `.stow` file; any directory containing a `.nonstow` file. It warns on stderr
**[probed]**:

```
WARNING: skipping target which was current stow directory <dir>
WARNING: skipping marked Stow directory <dir>
WARNING: skipping protected directory <dir>
```

**Asymmetry (ledger PL-04):** `unstow_contents` passes the *target* subdir to this check,
but `stow_contents` passes the *package* subdir. **[source]** Under `--dotfiles` those names
differ, so a package directory `dot-foo` stowing into a `.stow`-marked target `.foo`
**bypasses the protection**, while unstowing the same tree correctly refuses. **[probed]**

---

## 8. Output contract

### 8.1 Streams

- **stdout**: usage (`--help`, and *also* on every usage error), version. Nothing else, ever.
- **stderr**: all verbose/debug output, all warnings, all conflict reports, all errors.
- At verbosity 0 a successful run prints **nothing at all**. **[probed]**

`stow -n` without `-v` prints only `WARNING: in simulation mode so not modifying
filesystem.` on stderr — to see what *would* happen you need `-nv`. **[probed]**

### 8.2 Verbosity levels **[source: `Stow/Util.pm`]**

| Level | Content |
|---|---|
| 0 | errors only |
| ≥1 | operations: `LINK`/`UNLINK`/`MKDIR`/`RMDIR`/`MV` |
| ≥2 | operation exceptions: skipping, deferring, overriding, fixing invalid links |
| ≥3 | trace: stow/unstow/package/contents/node |
| ≥4 | debug helper routines |
| ≥5 | debug ignore lists |

Line counts at each level are **[probed]** (0→1, 1→2, 2→8, 3→12 stderr lines for a
one-file package under `-n`).

**Scope of the byte-parity guarantee — RULED (2026-07-09), ledger PL-11.** Levels 0–2 are a
scriptable contract and are **byte-exact**. Levels 3–5 emit Perl-internals-shaped traces
(`Stowing contents of ../st / pkg / . (cwd=/tmp/...)`) that expose absolute paths and call
structure; there, only **semantic equivalence** is owed.

The guarantee in testable form: **at any verbosity, the subsequence of lines that levels 0–2
would have emitted must match real stow byte-for-byte and in order.** Lines that only appear
at ≥3 are unconstrained. Byte-parity on 3–5 would pin gostow's internal call structure to
Perl's — fidelity nobody consumes, at the price of the freedom to be a different program
inside.

### 8.3 Message formats (verbatim)

Operations (level ≥1):

```
LINK: <path> => <dest>
LINK: <path> => <dest> (duplicates previous action)
LINK: <path> => <dest> (reverts previous action)
UNLINK: <path>
UNLINK: <path> (duplicates previous action)
UNLINK: <path> (reverts previous action)
MKDIR: <dir>
MKDIR: <dir> (duplicates previous action)
MKDIR: <dir> (reverts previous action)
RMDIR <dir>
RMDIR <dir> (duplicates previous action)
MV: <src> -> <dst>
```

`RMDIR` has **no colon**, unlike every sibling. This is reproducible and defined → **tier 1,
replicate**. Ledger PL-05. `do_rmdir`'s "reverts" branch prints `MKDIR ...`, not `RMDIR ...`
**[inferred, unprobed]** — and is on an unreachable-or-crashing path; see PL-06.

Conflicts (stderr, exit 1). Actions are reported `unstow` before `stow`; packages sorted;
messages within a package sorted. **[source; ordering partially probed]**

```
WARNING! <action>ing <package> would cause conflicts:
  * <message>
  * <message>
All operations aborted.
```

where `<action>` ∈ {`stow`, `unstow`} — yielding the literal words `stowing` / `unstowing`.
Conflict `<message>` values:

```
existing target is not owned by stow: <path>
existing target is stowed to a different package: <path> => <dest>
cannot stow <src> over existing target <path> since neither a link nor a directory and --adopt not specified
cannot stow non-directory <src> over existing directory target <path>
cannot stow directory <src> over existing non-directory target <path>
source is an absolute symlink <path> => <dest>
```

Fatal errors come in **two shapes**, and the difference is visible:

- `Stow::Util::error()` → `stow: ERROR: <message>` on stderr. **[probed]**
- a bare `die()` (only reachable from `.stowrc` handling) → `<message>` on stderr, with **no
  program-name prefix at all**. **[probed]**

Both are pinned to exit 2 in gostow (PL-07); both message forms are replicated verbatim.

Usage errors: `stow: <message>` on stderr followed by a **blank line**, then the full usage
block on **stdout**. A parse failure calls `usage('')`: the empty message prints nothing, but
still exits 1. **[probed]**

### 8.35 The `--gostow-` extension convention

gostow adds nothing to stow's option namespace. Anything gostow invents is prefixed
`--gostow-`, and three rules keep the parity mandate intact:

1. **Listed in `--help`**, because a flag nobody can discover is a flag nobody uses.
   No filtering is needed to keep that safe: help prose is not part of parity (§4.5), and
   what *is* checked — that gostow documents every option stow documents — is unharmed by
   gostow documenting more. `--gostow-help` prints the long form.
2. **Never abbreviated.** Extension options are `NoAbbrev` in `internal/getopt`, so they are
   absent from prefix matching *and* from ambiguity lists. `--g` therefore remains
   `Unknown option: g`, exactly as in real stow. Without this, adding an extension would
   silently redefine an abbreviation and change how existing argv parses.
3. **Forbidden in a parity fixture's argv.** `AssertSameAsOracle` fails loudly on a fixture
   whose argv contains `--gostow-`. Relaxing what is compared about *output* can be sound —
   both binaries still ran the same command, and §4.5 replaces the help comparison with
   sharper properties. Filtering an *argument* never is: it would compare
   gostow-with-the-flag against an oracle that never saw it, and the suite would quietly
   stop testing parity.

The consequence, and the reason the convention is safe: **for any command line that does not
literally contain `--gostow-`, gostow behaves exactly as GNU Stow** — the sole exception being
the two extension lines `--help` prints, which are additive and mechanically strippable.

The only extension today is `--gostow-fix`, the CLI face of `stow.Options.FixQuirks`.

### 8.36 `FixQuirks` — the library seam

`FixQuirks` (default false) abandons parity in a small, enumerated set of places where
stow's behaviour is a defect rather than a contract. It exists for consumers building *on*
the engine rather than replacing stow. Its scope is exactly:

| Ledger | Default (parity) | `FixQuirks` |
|---|---|---|
| PL-04 | `stow_contents` guards the *package* subdir, so `--dotfiles` bypasses `.stow` protection when stowing | guards the *target* subdir, as `unstow_contents` already does |
| PL-03 | `stow -- pkg` discards `pkg` | the arguments after `--` are package names |
| PL-02 | `.stowrc` has no comment syntax; an option-shaped word after `#` is silently honoured | `#` begins a comment, `\#` is a literal `#` |
| PL-05 | `RMDIR <dir>`, no colon | `RMDIR: <dir>` |

It deliberately does **not** touch PL-13 or PL-14, stow's two documented algorithmic bugs.
Those are work, not a flag. Nor does it touch anything in §2's tier-2 list, which gostow
never reproduces in the first place.

User-facing prose lives in `docs/DIVERGENCES.md`.

### 8.4 Colour (the sole additive liberty)

Colour is emitted **only** when the relevant stream is a TTY, and never changes byte content
on a pipe. `NO_COLOR` is respected. Colourised help is a TTY-only *rendering* of the exact
same text — never a re-layout. That constraint outlives the reason it was written down:
help prose is no longer byte-pinned to stow (§4.5), but colour must still be strippable
back to the plain bytes, because that is what makes it safe on every other stream.

**cobra's help/usage templates remain inapplicable**: gostow's help is a hand-written
block, not a generated one. The copied plumbing from dot-dagger reduces to the palette and
TTY-detection glue.

**Implemented (2026-07-09) in `internal/ui`.** Two structural choices turn the paragraph
above from a promise into a property of the code:

1. Colour is a **rendering pass over finished lines**, not coloured format strings at the
   call sites. A rule may only wrap an existing substring in an SGR escape, and the painted
   line is reassembled from the original bytes. `StripANSI(paint(s)) == s` is therefore an
   executable claim, asserted over every line shape gostow emits — paired with a test that
   each shape *is* painted, so the round trip cannot pass vacuously.
2. When colour is off, **the rendering pass does not run at all**: `ui.Writer.Write` hands
   the slice straight to the underlying stream. Code that never executes cannot perturb a
   pipe.

Both streams are wrapped once, in `cli.Run`. The engine writes plain text to `Options.Log`
and knows nothing about terminals, so `package stow` stays free of presentation — the same
line `bin/stow` and `Stow.pm` draw. Colour is disabled for a non-`*os.File`, a non-character
device, `NO_COLOR` (any non-empty value, per no-color.org) and `TERM=dumb`.

The differential harness captures the binary's output through pipes, so it observes
uncoloured bytes without having to ask; `TestNoColourOffATTY` pins that, and fails if any
call site ever writes an escape directly instead of going through `internal/ui`.

Palette: `LINK:` green, `UNLINK:` red, `MKDIR:`/`RMDIR` blue, `MV:` cyan, `ERROR:` bold red,
`WARNING!` bold yellow, conflict bullets red, the `--- ` trace prefix dim, help headings and
the identity line bold, option flags cyan, and gostow's own `--gostow-` flags magenta so a
reader can see at a glance which lines are not GNU Stow's.

---

## 9. Exit codes

| Situation | Real stow 2.4.1 | gostow | Tier |
|---|---|---|---|
| Success (incl. `-n`) | 0 | 0 | 1 |
| `--help`, `--version` | 0 | 0 | 1 |
| Unknown option | 1 | 1 | 1 |
| Ambiguous abbreviation | 1 | 1 | 1 |
| No packages given | 1 | 1 | 1 |
| `--dir`/`--target` not a directory | 1 | 1 | 1 |
| Conflicts detected | 1 | 1 | 1 |
| Fatal `error()` — bad package name, missing package | **2** | **2** | 2 |
| Fatal `die()` — undefined env var in `.stowrc` | **255** | **2** | 2 |

Perl's `die` exits with `$!` when errno is non-zero, else `$? >> 8`, else 255. So stow's
fatal exit status is **whatever errno happened to be left behind by the last failing
syscall**. Demonstrated **[probed]**:

```
perl -e 'open(F,"<","/definitely/missing"); die "boom\n"'  → 2   (ENOENT)
perl -e 'die "boom\n"'                                     → 255
```

That is textbook tier-2: non-reproducible, undefined, platform- and libc-dependent. **gostow
pins every fatal error to exit 2.** There is nothing to be faithful to. Ledger PL-07.

---

## 10. Parity Ledger

The audit trail of stow's quirks. Every discrepancy between "what stow does" and "what a
reasonable implementation would do" is logged here with its tier (§2.2) and ruling. This
table is a deliverable in its own right.

| ID | Finding | Evidence | Tier | Ruling |
|---|---|---|---|---|
| PL-01 | Man page says `.stowrc` search order is cwd-then-home; code reads home-then-cwd, concatenates, so cwd wins for scalar options. | probed | 1 | **Replicate the code.** Doc bug — report upstream. |
| PL-02 | `.stowrc` has no comment syntax. `#` and following words are parsed as package names and silently discarded, so comments "work" by accident. | probed | 3 | **Replicate.** Ledger; consider upstream request for real comment support. |
| PL-03 | `stow -- pkg` discards `pkg` (Getopt leaves it in the unread array) and fails with `No packages to stow or unstow`, exit 1. | probed | 3 | **Replicate.** Fails loudly and identically; no silent corruption. Revisit post-v1. Report upstream. |
| PL-04 | `stow_contents` passes the **package** subdir to `should_skip_target`, while `unstow_contents` passes the **target** subdir. Under `--dotfiles` these differ, so stowing **bypasses `.stow`/`.nonstow` protection** that unstowing honours. | probed (discriminating case: pkg `dot-foo` → target `.foo` marked with `.stow`) | 3 | **Replicate for v1.** Real protection bypass — **report upstream**; strong candidate for post-v1 divergence. |
| PL-05 | `RMDIR <dir>` printed without a colon; `LINK:`/`UNLINK:`/`MKDIR:`/`MV:` all have one. | probed | 1 | **Replicate**, and declare it: the engine has one operation table and one printer, where the missing colon is a `colon: false` field rather than a format string that reads as a typo. `FixQuirks` restores it. Report upstream. |
| PL-06 | `do_rmdir` reads `$self->{link_task_for}{$dir}` inside its `dir_task_for` branch → undef deref (crash); its "reverts" branch prints `MKDIR`, not `RMDIR`, and mutates the wrong table. | **probed (2026-07-09): unreachable** | 2 | **RULED: do not replicate — nothing to replicate.** The branch is dead code. `do_rmdir` is called only from `fold_tree`, only during `plan_unstow`, which wholly precedes `plan_stow`; `dir_task_for{$dir}` is therefore never set when it runs, and a directory can be folded at most once (after the first fold, `foldable` finds no links and returns early). Neither the crash nor the mislabelled `MKDIR (reverts)` can be reached from the CLI. gostow simply omits the branch. Report upstream. |
| PL-07 | Fatal exit status is errno-derived (2 after a failed stat, 255 on a clean `die`). | probed | 2 | **Do not replicate.** Pin fatal = 2. |
| PL-08 | `do_unlink` guards with `$self->{dir_task_for}{$file} eq 'create'` — comparing a hashref to a string. Always false; dead code. | source | 2 | **Do not replicate.** Implement the intended guard. Report upstream. |
| PL-09 | `cleanup_invalid_links` uses `if (not $link_dest)` after `readlink`. A symlink whose destination is the literal string `0` is falsy in Perl → spurious `Could not read link` fatal. | **probed (2026-07-09): confirmed** | 3 | **RULED: do not replicate.** A target symlink pointing at exactly `0` aborts the whole unstow with `stow: ERROR: Could not read link <path>`, exit 2, changing nothing — even though the link is perfectly readable and unrelated to the package. Destinations `00` and `0.0` unstow cleanly, so only `0` is affected. This is a Perl-falsiness bug (`not` where `defined` was meant), not behaviour; the same mistake sits in `foldable` and `unstow_link_node`. gostow uses `defined` and proceeds. Report upstream. |
| PL-10 | An **unreadable** `.stow-local-ignore` (exists but `open` fails) makes `get_ignore_regexps_from_file` return `undef`, disabling **all** ignore matching — including the built-in defaults and the self-ignore of `.stow-local-ignore` itself, which then gets stowed. | **probed (2026-07-09): confirmed** | 3 | **RULED: do not replicate; substitute a fatal error.** Reproduced exactly: `README.md` is stowed, `.stow-local-ignore` stows *itself*, exit 0, no warning at any verbosity below 5. `get_ignore_regexps` gates on `-e $file`, so an existing-but-unreadable file reaches `get_ignore_regexps_from_file`, whose failed `open` returns bare `undef` instead of falling through to the defaults; every `defined` guard in `ignore()` then fails. gostow treats an unreadable ignore file as a fatal error (exit 2) rather than silently ignoring nothing: a broken config must fail loudly, and silently falling back to the built-ins would be an equally unfounded guess at intent. Report upstream. |
| PL-11 | Verbosity ≥3 emits Perl-internals traces with absolute paths and call structure. | probed | 3 | **RULED (2026-07-09): scope the guarantee.** Byte-exact for levels **0–2**; at levels **3–5** only *semantic* equivalence is owed. Testable form: at any verbosity, the subsequence of lines that levels 0–2 would have emitted must match byte-for-byte and in order; additional trace lines are unconstrained. Levels 0–2 are the scriptable contract. Byte-parity on 3–5 would pin gostow's internal call structure to Perl's, buying fidelity nobody consumes at the price of the freedom to be a different program inside. |
| PL-12 | `--version` prints `basename($0) (GNU Stow) version 2.4.1`, leaving no room for gostow's own build version. | source | — | **RULED (2026-07-09): diverge.** `--version` reports gostow's version, naming the conformant stow version alongside: `gostow 0.1.0 (GNU Stow 2.4.1 compatible)`. Parity is owed to behaviour scripts depend on, not to self-identification. The only intentional output divergence. §4.4 |
| PL-13 | Documented known bug: the **empty-directory problem** (unstowing `quux` removes `target/bar` even though `foo/bar` needs it). | stow man page | 3 | **Replicate in v1.** Being better than stow is a different project. |
| PL-14 | Documented known bug: tree-folding symlinks pointing into a *different* stow directory fail to split open. | stow man page | 3 | **Replicate in v1.** |
| PL-15 | Perl regex vs RE2: `$` in Perl matches before a trailing newline; Go's `$` matches end-of-text only. Filenames may legally contain `\n`. | source analysis | 3 | Ledger. Decide during ignore-matcher implementation. |
| PL-16 | `--no-folding` is a real flag but is **absent from `--help`** (present in the man page). | probed | 1 | **RULED (2026-07-09): fix it.** Originally *replicate the omission*, because the help block was transcribed byte for byte. §4.5 removed that transcription — help prose is not part of parity, on both licensing and correctness grounds — so nothing is owed to an omission. gostow documents `--no-folding`. A hermetic test derives the expected flag list from the parser's own option table, so no future flag can be added and left undocumented; that is the check whose absence produced this entry upstream. |
| PL-19 | `Value "abc" invalid for option verbose (...)` is emitted by **`Getopt::Long`, not by stow**, and its wording changed in Getopt::Long 2.55: 2.54 (Ubuntu 24.04, perl 5.38) says `(number expected)`, 2.58 (perl 5.40) says `(integer number expected)`. The byte is therefore a function of the installed Perl, not of the pinned stow 2.4.1. | **probed (2026-07-09)**, on two Perls | 2 | **Cannot replicate — the referent is undefined.** "stow 2.4.1's behaviour" does not determine this string. gostow pins the **current upstream wording** (`integer number expected`), which is what Getopt::Long has said since 2.55 and will keep saying. The differential suite folds the two spellings together, and *only* those two. |
| PL-18 | `Stow.pm` interpolates `$ENV{HOME}` into a regex unescaped (`$msg =~ s!$ENV{HOME}(/|$)!~$1!g`, `Stow.pm:409` and `:759`), to abbreviate the home directory in a trace message. A `$HOME` containing a regex metacharacter therefore **kills stow before it does any work** — at *every* verbosity, because the substitution runs before the `debug()` guard. `HOME=/tmp/ho(me stow pkg` → `Unmatched ( in regex ... at Stow.pm line 409.`, exit 2, nothing stowed. | **probed (2026-07-09)** | 2 | **Do not replicate.** A crash, and undefined for any user whose home path contains `(`, `[`, `+`, `*`, `?`, `\` … gostow never builds that regex: the message it decorates exists only at verbosity ≥3, where PL-11 owes semantic equivalence, not bytes. **Report upstream** — this is the most serious defect the audit found. |
| PL-17 | stow's program name is `basename($0)`, not a constant: symlink stow to `mystow` and the usage errors, the `--help` synopsis and the version banner all say `mystow`. | probed | 1 | **Replicate the mechanism.** gostow derives the program name from `os.Args[0]` exactly as stow does, so `stow: ERROR: ...` and the synopsis line are byte-exact when gostow is installed under the name `stow` — which is how it is used as a drop-in. The **identity line alone** is fixed to `gostow`, per PL-12: it names the tool, not the invocation. This is what lets the differential harness compare stderr byte-for-byte (it runs gostow's binary named `stow`). |

**Upstream bug reports to file:** PL-01, PL-03, PL-04, PL-05, PL-06, PL-08, PL-09, PL-10, **PL-18** (highest severity: a `$HOME` containing a regex metacharacter makes stow unusable).

---

## 11. Ignore handling

### 11.1 Precedence — the three sources are **exclusive, not merged**

`get_ignore_regexps` returns the **first** source that exists **[source]**, and this is
counter-intuitive enough to be worth stating loudly:

1. `<stow_dir>/<package>/.stow-local-ignore`
2. `$HOME/.stow-global-ignore`
3. the built-in default list

Creating `~/.stow-global-ignore` therefore **discards the built-in defaults entirely** — a
`README.md` that stow would normally ignore becomes stowed. Creating a package-local
`.stow-local-ignore` discards the global file *and* the defaults. Both **[probed]**.

Results are memoized per file path for the lifetime of the process. **[source]**

`--ignore` patterns are additive and are checked **before** any of the three sources. **[source]**

### 11.2 File format **[source]**

Per line: strip leading/trailing whitespace; skip if empty or starting with `#`; strip a
trailing comment matching `\s+#.+`; unescape `\#` → `#`. Duplicate patterns collapse.

The regexp `^/\.stow-local-ignore$` is **always** appended, so a local ignore file never
stows itself.

Patterns are then partitioned by whether they contain a `/`:

- **no `/`** → *segment* regexps, joined by `|` and matched as `^(...)$` against the node's **basename**.
- **contains `/`** → *path* regexps, joined by `|` and matched as `(^|/)(...)(/|$)` against `"/" + <target path>`.

### 11.3 Built-in defaults (verbatim, post-processing) **[source: `Stow.pm` `__DATA__`]**

Segment patterns:
`RCS`, `.+,v`, `CVS`, `\.#.+`, `\.cvsignore`, `\.svn`, `_darcs`, `\.hg`, `\.git`,
`\.gitignore`, `\.gitmodules`, `.+~`, `#.*#`

Path patterns:
`^/README.*`, `^/LICENSE.*`, `^/COPYING`, plus the always-appended `^/\.stow-local-ignore$`

Note `\.\#.+` and `\#.*\#` in the source become `\.#.+` and `#.*#` after `\#`-unescaping.
All are RE2-compatible.

### 11.4 `--ignore` / `--defer` / `--override` anchoring

| Option | Compiled as | Matched against | Meaning |
|---|---|---|---|
| `--ignore=R` | `(R)\z` | target node path, relative to target | **suffix** match |
| `--defer=R` | `\A(R)` | target node path | **prefix** match |
| `--override=R` | `\A(R)` | target node path | **prefix** match |

Consequences **[probed]**: `--ignore=log` ignores anything ending in `log`.
`--ignore='^skip'` compiles to `(^skip)\z` and therefore matches only the exact string
`skip` — **not** `skip.log`. `--ignore='sub/deep\.log'` matches the full relative path.

### 11.5 Regex engine

Ruling: **RE2 first, escape hatch later.** Use Go's stdlib `regexp` (linear time, no ReDoS,
no dependency). All built-in defaults are RE2-compatible. If a pattern fails to compile,
fail cleanly at parse time with a clear message rather than silently mismatching. If a real
user pattern ever needs lookaround or backreferences, fall back to a vendored backtracking
engine **for that pattern only**, keeping the common path fast.

See PL-15 for the `$`-vs-newline divergence.

---

## 12. Open items

All ledger items are now ruled. What remains:

Nothing. Every ledger item is ruled, every slice in `TEST-PLAN.md` §5 is implemented, and
each is pinned by a differential test against the pinned oracle.

What that claim does **not** mean: parity is evidenced, not proved. The suite compares 6307
argv vectors against real `Getopt::Long`, 1140 ignore verdicts against `Stow.pm`'s own
`ignore()`, and 60 whole-invocation fixtures against the real binary — stdout bytes, stderr
bytes, exit code and resulting tree. A fixture nobody wrote is a behaviour nobody checked.

### Settled (2026-07-09)

- **`chkstow` is out of scope for v1.** The parity mandate is owed by the `stow` command:
  its flags, its output, its symlink farm. `chkstow` is a *different program* with its own
  CLI, sharing no code path with the engine, consumed neither by dstow nor by any `.stowrc`
  or script that drives `stow`. Shipping it would be cargo-culting the tarball's contents
  rather than serving an install. Revisit if a real use appears.
- **`--compat` now has a discriminating fixture.** Rename a package file and leave the old
  target link dangling: both modes end with the same tree, but walking the *package* tree
  never visits the stale link (it is swept up afterwards by `cleanup_invalid_links`) while
  walking the *target* tree finds it directly and reports it as an invalid link into a stow
  directory. compat never runs `cleanup_invalid_links` at all.
- **PL-04 is pinned by a test**, and is worse than first recorded: stowing `dot-foo` into a
  `.stow`-marked `.foo` not only bypasses the protection, it *creates* `.foo/bar` — which
  the matching unstow then refuses to remove, stranding the file permanently.
- **PL-15** ruled during the ignore-matcher implementation: keep RE2 semantics.

- **PL-18** probed and ruled: `$HOME` is interpolated into a regex unescaped, so a home
  directory containing a regex metacharacter crashes real stow at any verbosity. Not
  replicated.

- Go module path is `github.com/rocne/gostow`; the engine is `package stow` at `stow/`.
- **PL-06** probed unreachable; **PL-09** and **PL-10** probed and confirmed as genuine bugs,
  both ruled *do not replicate*.
- **PL-11** ruled: byte-exact at verbosity 0–2, semantic equivalence at 3–5.
- **PL-12** ruled: `--version` reports gostow's own version.
- **PL-17** ruled: program name follows `basename($0)`, identity line excepted.
