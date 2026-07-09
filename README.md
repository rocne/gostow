# gostow

**GNU Stow, reimplemented in Go.** A single static binary, no Perl.

gostow is a drop-in replacement for [GNU Stow](https://www.gnu.org/software/stow/) 2.4.1.
Same flags, same output, same exit codes, same symlinks. Your existing scripts and
`.stowrc` files do not know the difference — and that is checked, not asserted: the test
suite runs the real `stow` binary alongside gostow and compares stdout, stderr, exit status
and the resulting directory tree.

(Two things are deliberately gostow's own: it colours a terminal, and `--help` is written
in gostow's words. Neither reaches a script. See [what's different](#whats-different).)

```console
$ gostow --dotfiles -t ~ vim
$ gostow -D vim            # unstow
$ gostow -R vim            # restow
```

## Install

Download a binary for linux or macOS (amd64 or arm64) from the
[releases page](https://github.com/rocne/gostow/releases), or build from source:

```console
$ go build -o gostow ./cmd/gostow
```

To use it as a true drop-in, install it as `stow`. gostow takes its program name from
`argv[0]`, exactly as stow does, so its usage errors and its `--help` synopsis will say
`stow`:

```console
$ install -m755 gostow /usr/local/bin/stow
```

## What's different

Almost nothing, on purpose. The exhaustive list is in **[docs/DIVERGENCES.md](docs/DIVERGENCES.md)**,
and `gostow --gostow-help` summarises it at the terminal. In brief:

- **gostow does not reproduce stow's crashes.** Real stow dies before doing any work if your
  home directory contains a `(` or a `[`; it aborts an entire unstow if some unrelated
  symlink points at the text `0`; and it silently disables *all* ignore rules if
  `.stow-local-ignore` exists but cannot be read. gostow does none of these.
- **gostow colours its output on a terminal**, and only on a terminal. Pipe it, redirect it,
  or set `NO_COLOR`, and every byte is stow's. *Byte-compatible on a pipe, prettier on a TTY.*
- **`gostow --help` is gostow's own prose**, and documents `--no-folding` — a real, working
  flag that stow's help has never mentioned. Option *parsing*, the usage diagnostic on
  stderr, and exit codes are all still byte-exact.
- **`--gostow-fix`** turns off the remaining bug-compatibility — stow's `.stowrc` having no
  comment syntax, `stow -- pkg` silently discarding `pkg`, `RMDIR` printing without a colon,
  and a real `--dotfiles` protection bypass. Everything gostow adds is prefixed `--gostow-`
  and cannot be abbreviated, so no command line GNU Stow accepts is parsed differently here.

Two of stow's own documented bugs — the empty-directory problem, and folding across stow
directories — are reproduced faithfully and are not yet fixed by `--gostow-fix`.

## As a library

The engine is a deep module with a narrow surface, consumed by the sibling tool `dstow`:

```go
import "github.com/rocne/gostow/stow"

_, err := stow.Apply(stow.Options{
    Dir:       "/home/me/dotfiles",
    Target:    "/home/me",
    Fold:      true,
    Dotfiles:  true,
    FixQuirks: true, // stow's engine, without stow's defects
}, stow.Request{Action: stow.ActionStow, Packages: []string{"vim"}})
```

`Apply` plans every package first, collects all conflicts, and only then touches the disk:
an invocation is all-or-nothing. `FixQuirks` defaults to false because gostow's promise is
to *be* stow; a consumer building something better on top should turn it on.

## Development

```console
$ go build ./... && go vet ./... && go test ./...   # hermetic: no Perl required
$ golangci-lint run ./...
```

The specification is **[docs/SPEC.md](docs/SPEC.md)**, and it is executable: GNU Stow 2.4.1
is the conformance referent, and where the spec and the real binary disagree, the binary
wins. Install the pinned oracle and run the differential suite with:

```console
$ PREFIX=$PWD/.oracle bash test/install-stow-oracle.sh
$ PATH=$PWD/.oracle/bin:$PATH go test -tags oracle ./...
```

That compares 6307 argv vectors against real `Getopt::Long`, 1216 ignore verdicts against
`Stow.pm`'s own `ignore()`, and 62 differential fixtures — 20 engine-level, 42 driving the
whole binary — against real stow 2.4.1. Do **not** `apt install stow`: Ubuntu 24.04 ships
2.3.1, which would silently redefine the spec.

`docs/SPEC.md` §10 is the **Parity Ledger** — every quirk found in stow's source, whether it
is a contract or a bug, and what gostow does about it. Nine of them are bugs owed upstream.

## Status

The engine is complete and the exported API is stable under semver.

Parity is *evidenced*, not proved — a fixture nobody wrote is a behaviour nobody checked.
What the evidence covers is written down: `docs/SPEC.md` §10 rules on every stow quirk
found, `docs/TEST-PLAN.md` describes the layers, and the differential suite runs the real
binary on every change.

## Licence

gostow is **MIT** licensed. See `LICENSE`.

It is an independent reimplementation and shares no source code with GNU Stow, which is
GPLv3-or-later. It does reproduce GNU Stow's *behaviour* — option names, parsing, exit
codes, diagnostic messages, and the built-in ignore patterns — because that is what a
drop-in replacement is. `NOTICE` records exactly what is reproduced and why.

gostow's `--help` is written in gostow's own words. GNU Stow's
[manual](https://www.gnu.org/software/stow/manual/) is the authority on what the shared
options mean, and it describes gostow exactly.
