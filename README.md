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

Packages are published for linux and macOS on amd64 and arm64. Every method below installs
the binary as **`gostow`**; see [Using it as a drop-in](#using-it-as-a-drop-in) to make it
answer to `stow`.

### macOS — Homebrew

```console
$ brew install --cask rocne/tap/gostow
```

**A cask, not a formula, and `gostow` still lands on your `PATH`.** Casks are no longer just
GUI apps: the cask's `binary "gostow"` stanza symlinks the executable into
`$(brew --prefix)/bin` exactly as a formula would. It is a cask because Homebrew's
[Acceptable Formulae](https://docs.brew.sh/Acceptable-Formulae) policy says *"binary-only
formulae should go in homebrew/cask"* — formulae are meant to build from source, and gostow
ships pre-compiled binaries. GoReleaser deprecated its formula output for the same reason.

Homebrew treats Cask as a macOS feature. On Linux, use the tarball or `go install` below.

### Debian, Ubuntu — apt

```console
$ curl -1sLf https://dl.cloudsmith.io/public/rocne/releases/setup.deb.sh | sudo -E bash
$ sudo apt-get update && sudo apt-get install -y gostow
```

### Fedora, RHEL — dnf

```console
$ curl -1sLf https://dl.cloudsmith.io/public/rocne/releases/setup.rpm.sh | sudo -E bash
$ sudo dnf install -y gostow
```

Each is two steps: **register the repository, then install from it with your usual package
manager.** The first line fetches Cloudsmith's setup script and runs it as root — it writes
an apt source list (or a dnf `.repo` file) and imports the repository's signing key.
`-1sLf` is curl for *TLSv1.0 or better, quiet, follow redirects, and fail loudly on an HTTP
error rather than piping an error page into a shell*; `sudo -E` keeps your environment so
the script can see a proxy if you have one.

Piping a script into a root shell is a real thing to be uneasy about. You do not have to:

```console
$ curl -1sLf -O https://dl.cloudsmith.io/public/rocne/releases/setup.deb.sh
$ less setup.deb.sh          # it adds one source list and imports one key
$ sudo -E bash setup.deb.sh
```

The repository is public — [`rocne/releases`](https://cloudsmith.io/~rocne/repos/releases/) —
and both the packages and the repository index are GPG-signed. Both public keys are attached
to every GitHub release: `gostow-signing-key.asc` signs the packages,
`gostow-repo-signing-key.asc` signs the index.

Every release is installed from these repositories, on Ubuntu 24.04 and Fedora 41, by CI
before it is announced. If the instructions above stop working, the release fails rather
than these docs quietly going stale.

### Any linux or macOS — tarball

```console
$ VER=v0.1.0 OS=linux ARCH=amd64        # OS: linux|darwin   ARCH: amd64|arm64
$ base=https://github.com/rocne/gostow/releases/download/$VER

$ curl -fsSLO $base/gostow_${VER}_${OS}_${ARCH}.tar.gz
$ curl -fsSLO $base/gostow_${VER}_checksums.txt
$ sha256sum -c --ignore-missing gostow_${VER}_checksums.txt

$ tar xzf gostow_${VER}_${OS}_${ARCH}.tar.gz
$ sudo install -m755 gostow /usr/local/bin/gostow
```

### With the Go toolchain

```console
$ go install github.com/rocne/gostow/cmd/gostow@latest
```

### From source

```console
$ git clone https://github.com/rocne/gostow && cd gostow
$ go build -o gostow ./cmd/gostow
```

### Verifying a release

Every archive carries a [sigstore](https://www.sigstore.dev/) build provenance attestation,
and `checksums.txt` is signed with a keyless cosign signature (`.sig` + `.pem`).

```console
$ gh attestation verify gostow_v0.1.0_linux_amd64.tar.gz \
    --repo rocne/gostow --signer-repo rocne/release-ci
```

`--signer-repo` is required, and its absence is easy to misread: the artifacts are built and
signed by a reusable workflow living in `rocne/release-ci`, so that — not `rocne/gostow` —
is the signing identity. Omit it and the command fails with a bare
`Error: verifying with issuer "sigstore.dev"`, which looks like a bad signature but is a
wrong invocation.

### Using it as a drop-in

gostow takes its program name from `argv[0]`, exactly as stow does, so invoked as `stow` its
usage errors and its `--help` synopsis say `stow`. Point the name at it:

```console
$ sudo ln -sf "$(command -v gostow)" /usr/local/bin/stow
$ stow --version
gostow 0.1.0 (GNU Stow 2.4.1 compatible)
```

If GNU Stow is also installed, make sure `/usr/local/bin` precedes `/usr/bin` on your
`PATH`, or the Perl one wins.

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

The hermetic suite is not a weaker suite. It replays the differential fixtures against frozen
recordings of what real stow 2.4.1 did — stdout, stderr, exit code and the resulting tree.

The specification is **[docs/SPEC.md](docs/SPEC.md)**, and it is executable: GNU Stow 2.4.1
is the conformance referent, and where the spec and the real binary disagree, the binary
wins. Install the pinned oracle and run the differential suite with:

```console
$ PREFIX=$PWD/.oracle bash test/install-stow-oracle.sh
$ PATH=$PWD/.oracle/bin:$PATH go test -count=1 -tags oracle ./...
```

That compares argv vectors against real `Getopt::Long`, ignore verdicts against `Stow.pm`'s
own `ignore()`, `parent` and `join_paths` against `Stow::Util`, errno strings against Perl's
`$!`, and whole-invocation fixtures against the real binary. Every one of those tests prints
its own count — read it there, not here.

`-count=1` is not optional: the harness reaches its oracles through subprocesses, and Go's
test cache cannot see a subprocess's inputs. Do **not** `apt install stow` either: Ubuntu
24.04 ships 2.3.1, which would silently redefine the spec.

Regenerate the frozen recordings after a deliberate change:

```console
$ PATH=$PWD/.oracle/bin:$PATH go test -count=1 -tags oracle ./internal/conformance/ -update-goldens
```

`docs/SPEC.md` §10 is the **Parity Ledger** — every quirk found in stow's source, whether it
is a contract or a bug, and what gostow does about it. Ten of them are bugs owed upstream.
`docs/audit-2026-07-10.md` is an external audit of this codebase; six of its findings were
parity bugs, and all six are fixed.

## Status

Pre-1.0, deliberately. A `v1` tag is a promise of parity that is evidenced but not proved:
a fixture nobody wrote is a behaviour nobody checked. See `docs/TEST-PLAN.md`.

## Licence

gostow is **MIT** licensed. See `LICENSE`.

It is an independent reimplementation and shares no source code with GNU Stow, which is
GPLv3-or-later. It does reproduce GNU Stow's *behaviour* — option names, parsing, exit
codes, diagnostic messages, and the built-in ignore patterns — because that is what a
drop-in replacement is. `NOTICE` records exactly what is reproduced and why.

gostow's `--help` is written in gostow's own words. GNU Stow's
[manual](https://www.gnu.org/software/stow/manual/) is the authority on what the shared
options mean, and it describes gostow exactly.
