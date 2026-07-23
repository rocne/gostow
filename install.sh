#!/usr/bin/env sh
# install.sh — put a verified binary on a bare machine. That is the whole job
# (DESIGN.md §6.10): package managers and mise own routine upgrades.
#
# Canonical source: rocne/release-ci, installer/install.sh. Vendored to each
# consumer's repo root with the config block below set; everything below the
# block is byte-identical across consumers (D5/D6). The canonical copy has an
# empty config block and aborts with a usage error (D22).
#
# One-liner (vendored copies; sh is dash on Debian/Ubuntu — this file is POSIX):
#   curl -fsSL https://raw.githubusercontent.com/<owner>/<repo>/main/install.sh | sh
# With flags:
#   curl -fsSL .../install.sh | sh -s -- --version v0.2.0
# Local:
#   ./install.sh --help
#
# Artifact-shape contract (D34) — what this script consumes; asserted by each
# consumer's release-dryrun, produced by its GoReleaser config:
#   <bin>_<tag>_<os>_<arch>.tar.gz          the archive, binary at its root
#   <bin>_<tag>_checksums.txt (+ .sigstore.json) checksums + cosign bundle
#   man/ and completions/ inside the archive when the tool ships them
#
# Positioning, honestly (§6.9): presence-checking by default is a minority
# stance (5 of 15 surveyed installers; mise's is opt-in). Every checking
# installer speaks on the no-op path — our --silent is our own extension.
# ~/.local/bin as default is mise's choice, not the majority's (most use a
# tool-specific dir). And a checksum whose file rides the same origin as the
# artifact is integrity, not authenticity: F4 defends against corruption and
# truncation. Authenticity is cosign (F5), verified whenever cosign is
# present, enforced by --require-signature.
#
# Structure (D31): all logic lives in functions; the only top-level statements
# are the shell options, the config block, and the final `main "$@"`. A
# truncated `curl | sh` therefore executes nothing rather than a prefix.

set -eu

# ---- vendored config (per-consumer; everything below is canonical) ----
REPO="rocne/gostow"                          # GitHub slug, e.g. rocne/dot-dagger. Set at vendor time.
BIN=""                            # installed binary name; only set if it diverges from
                                  # the repo name (e.g. rocne/dot-dagger -> dotd).
SIGNER_REPO="rocne/release-ci"   # Fulcio identity the release is signed as (D25)
# ---- end vendored config ----

BIN="${BIN:-${REPO##*/}}"

# Output levels (D10): silent=0 quiet=1 normal=2 verbose=3. The exit code is
# unconditional and level-independent (F6); only the *message* varies. All
# human output goes to stderr at every level — stdout stays empty, so no
# `curl | sh` pipeline ever mistakes chatter for data. No color and no
# progress bars are emitted at any level, which honors NO_COLOR trivially and
# needs no TTY check; if color is ever added, gate it on both.

usage() {
  cat >&2 <<EOF
install.sh — download and install ${BIN:-<bin>} from ${REPO:-<repo>} releases

Usage:
  curl -fsSL https://raw.githubusercontent.com/${REPO:-<owner>/<repo>}/main/install.sh | sh
  curl -fsSL .../install.sh | sh -s -- [flags]
  ./install.sh [flags]

Flags:
  --version vX.Y.Z     ensure exactly this version: already installed at that
                       version → exit 0; otherwise install it. Does NOT imply
                       --force.
  --force              install unconditionally, over anything present
  --install-dir <dir>  install directory (default: ~/.local/bin)
  --os <os>            override OS detection (linux, darwin)
  --arch <arch>        override architecture detection (amd64, arm64)
  --bin-only           binary only; skip man pages and completions
  --require-signature  make cosign verification mandatory instead of
                       opportunistic
  --dry-run            print what would be done, then exit without installing
  -v, --verbose        more detail        | -q, --quiet   changes and failures only
  --silent             abort messages only; exit codes are never suppressed
  -h, --help           this text

Environment (namespaced from the binary name, e.g. GOSTOW_*):
  <BIN>_INSTALL_DIR        install directory (--install-dir wins over it)
  <BIN>_INSTALL_LOG_LEVEL  verbose | normal | quiet | silent (flags win)
  XDG_BIN_HOME              honored below both (uv's convention)

Behavior:
  Already installed and no flags → one status line, exit 0, no network.
  Found elsewhere on PATH (apt/brew) → not shadowed; says so, exit 0.
  Checksum verification is mandatory: no sha256sum/shasum → abort.
  cosign, when present, verifies the release signature (Sigstore). The release
  is signed as a Sigstore bundle (new format), so verification needs cosign v3+;
  an older cosign is treated like a missing signature (skipped, or aborts under
  --require-signature).

Exit codes: 0 success or no-op · 1 runtime failure · 2 usage error
EOF
}

# ---- output ----------------------------------------------------------------

LOG_LEVEL_N=2

emit() { printf '%s\n' "$1" >&2; }

log_error()  { emit "error: $1"; }                                # every level
log_hint()   { emit "hint: $1"; }                                 # rides errors
log_change() { if [ "$LOG_LEVEL_N" -ge 1 ]; then emit "$1"; fi; } # quiet+
log_warn()   { if [ "$LOG_LEVEL_N" -ge 1 ]; then emit "$1"; fi; } # quiet+
log_info()   { if [ "$LOG_LEVEL_N" -ge 2 ]; then emit "$1"; fi; } # normal+
log_detail() { if [ "$LOG_LEVEL_N" -ge 3 ]; then emit "$1"; fi; } # verbose

# die MESSAGE [HINT] [EXIT_CODE]
die() {
  log_error "$1"
  if [ -n "${2:-}" ]; then log_hint "$2"; fi
  exit "${3:-1}"
}

set_level() {
  case "$1" in
    silent)  LOG_LEVEL_N=0 ;;
    quiet)   LOG_LEVEL_N=1 ;;
    normal)  LOG_LEVEL_N=2 ;;
    verbose) LOG_LEVEL_N=3 ;;
    *) die "unknown log level: $1" "valid: verbose, normal, quiet, silent" 2 ;;
  esac
}

# ---- argument and environment handling -------------------------------------

REQ_VERSION=""
FORCE=0
FLAG_INSTALL_DIR=""
OS=""
ARCH=""
BIN_ONLY=0
REQUIRE_SIG=0
DRY_RUN=0
LEVEL_FROM_FLAG=""

# need_arg FLAG REMAINING_ARGC VALUE — flags take space-separated values only.
need_arg() {
  if [ "$2" -lt 2 ] || [ -z "$3" ]; then
    die "$1 requires an argument" "see --help" 2
  fi
}

parse_args() {
  while [ $# -gt 0 ]; do
    case "$1" in
      --version)     need_arg "$1" $# "${2:-}"; REQ_VERSION="$2"; shift 2 ;;
      --force)       FORCE=1; shift ;;
      --install-dir) need_arg "$1" $# "${2:-}"; FLAG_INSTALL_DIR="$2"; shift 2 ;;
      --os)          need_arg "$1" $# "${2:-}"; OS="$2"; shift 2 ;;
      --arch)        need_arg "$1" $# "${2:-}"; ARCH="$2"; shift 2 ;;
      --bin-only)    BIN_ONLY=1; shift ;;
      --require-signature) REQUIRE_SIG=1; shift ;;
      --dry-run)     DRY_RUN=1; shift ;;
      -v|--verbose)  LEVEL_FROM_FLAG=verbose; shift ;;
      -q|--quiet)    LEVEL_FROM_FLAG=quiet; shift ;;
      --silent)      LEVEL_FROM_FLAG=silent; shift ;;
      -h|--help)     usage; exit 0 ;;
      *) die "unknown argument: $1" "see --help" 2 ;;
    esac
  done

  if [ -n "$REQ_VERSION" ]; then
    REQ_VERSION="${REQ_VERSION#v}"
    if ! printf '%s' "$REQ_VERSION" \
      | grep -Eq '^[0-9]+\.[0-9]+\.[0-9]+([-+][0-9A-Za-z.-]+)?$'; then
      die "--version wants a version like v0.2.0, got: $REQ_VERSION" "see --help" 2
    fi
  fi
}

require_vendored_config() {
  if [ -z "$REPO" ] || [ -z "$BIN" ]; then
    die "REPO/BIN are unset — this is the canonical source, not an installer" \
      "vendor this script into a consumer repo and set the config block" 2
  fi
}

# The env vars are namespaced from the binary name (D8): a bare INSTALL_DIR
# exported for any unrelated purpose would be silently absorbed. eval is
# confined to reading ${<PREFIX>_...} where PREFIX is [A-Z0-9_] derived from
# BIN.
ENV_INSTALL_DIR=""

read_environment() {
  env_prefix=$(printf '%s' "$BIN" | tr '[:lower:]-' '[:upper:]_' | tr -cd 'A-Z0-9_')
  eval "ENV_INSTALL_DIR=\${${env_prefix}_INSTALL_DIR:-}"
  eval "env_log_level=\${${env_prefix}_INSTALL_LOG_LEVEL:-}"
  # Flags win over the environment.
  if [ -n "$LEVEL_FROM_FLAG" ]; then
    set_level "$LEVEL_FROM_FLAG"
  elif [ -n "$env_log_level" ]; then
    set_level "$env_log_level"
  fi
}

# Install dir precedence (§6.5): flag, namespaced env, XDG_BIN_HOME (uv's
# convention — 1 of 15 surveyed tools), then the contractual default (F3).
# The *default* is what rc snippets bake; the dir itself is tunable (D9).
INSTALL_DIR=""

resolve_install_dir() {
  if [ -n "$FLAG_INSTALL_DIR" ]; then INSTALL_DIR="$FLAG_INSTALL_DIR"
  elif [ -n "$ENV_INSTALL_DIR" ]; then INSTALL_DIR="$ENV_INSTALL_DIR"
  elif [ -n "${XDG_BIN_HOME:-}" ]; then INSTALL_DIR="$XDG_BIN_HOME"
  else INSTALL_DIR="$HOME/.local/bin"
  fi
}

# ---- prerequisites and platform --------------------------------------------

SHA_TOOL=""

require_tools() {
  if ! command -v curl >/dev/null 2>&1; then
    die "curl not found" "install curl, then re-run"
  fi
  # F4: checksum verification is mandatory, so its absence aborts before any
  # download. There is deliberately no skip flag — that would be an integrity
  # bypass one typo away.
  if command -v sha256sum >/dev/null 2>&1; then SHA_TOOL=sha256sum
  elif command -v shasum >/dev/null 2>&1; then SHA_TOOL=shasum
  else
    die "no sha256sum or shasum found — cannot verify download integrity" \
      "install coreutils (sha256sum) or perl (shasum), then re-run"
  fi
}

detect_platform() {
  if [ -z "$OS" ]; then
    case "$(uname -s)" in
      Linux)  OS=linux ;;
      Darwin) OS=darwin ;;
      *) die "unsupported OS: $(uname -s)" "use --os to override" ;;
    esac
  fi
  if [ -z "$ARCH" ]; then
    case "$(uname -m)" in
      x86_64|amd64)  ARCH=amd64 ;;
      arm64|aarch64) ARCH=arm64 ;;
      *) die "unsupported architecture: $(uname -m)" "use --arch to override" ;;
    esac
  fi
}

fetch() { curl -fsSL --proto '=https' --tlsv1.2 --retry 3 "$@"; }

# ---- presence check (F1, D16, D28) -----------------------------------------

# D30's parse rule: the first line of `$bin --version` output contains the
# binary's own version as the first semver-shaped token on that line. Prints the
# token, or nothing when the binary can't run or nothing parses — D28 defines
# that degradation as "unsatisfied": ensure then installs, which converges and
# repairs a broken binary as a side effect.
version_of() {
  # </dev/null: never let the probed binary read stdin — under `curl | sh`,
  # stdin is this script's own delivery channel.
  ver_line=$("$1" --version </dev/null 2>/dev/null | head -n 1) || ver_line=""
  printf '%s' "$ver_line" \
    | grep -Eo '[0-9]+\.[0-9]+\.[0-9]+([-+][0-9A-Za-z.-]+)?' | head -n 1 || true
}

# Both checks, install path first (D16). Install-path-first converges: a custom
# dir off PATH would reinstall forever under `command -v` alone (dstow's B6 ask,
# correct only in the rc snippet where PATH is already prepended). The wider
# PATH is checked second so an apt/brew-managed copy is never silently
# shadowed (mise checks only the install path and gets that case wrong).
# Presence-checking at all is our deliberate minority stance (§6.9).
presence_check() {
  target="$INSTALL_DIR/$BIN"
  # A directory (or any non-file) at the target can never be installed over:
  # mv would move the binary *inside* it and report success falsely, and every
  # later run would call the directory "already installed" — a permanent wedge.
  # This holds under --force too, which is why it precedes the force return.
  if [ -e "$target" ] && [ ! -f "$target" ]; then
    die "$target exists but is not a regular file — cannot install over it" \
        "remove it, or pass --install-dir for a different location"
  fi
  if [ "$FORCE" = 1 ]; then return 0; fi

  if [ -e "$target" ]; then
    have=$(version_of "$target")
    if [ -z "$REQ_VERSION" ]; then
      log_info "$BIN ${have:+v$have }already installed at $target; use --force to reinstall"
      exit 0
    fi
    if [ "$have" = "$REQ_VERSION" ]; then
      log_info "$BIN v$have already installed at $target"
      exit 0
    fi
    log_detail "$target reports ${have:+v$have}${have:-no parseable version}; ensuring v$REQ_VERSION"
    return 0
  fi

  found=$(command -v "$BIN" 2>/dev/null) || found=""
  if [ -n "$found" ]; then
    have=$(version_of "$found")
    if [ -z "$REQ_VERSION" ]; then
      log_info "$BIN ${have:+v$have }found at $found; not shadowing it (use --force to install to $INSTALL_DIR)"
      exit 0
    fi
    if [ "$have" = "$REQ_VERSION" ]; then
      log_info "$BIN v$have found at $found"
      exit 0
    fi
    # An explicit --version the found copy does not satisfy installs anyway:
    # the user asked for a version, not a location. Say what gets shadowed.
    log_warn "$BIN ${have:+v$have }at $found does not satisfy v$REQ_VERSION; installing to $INSTALL_DIR (one will shadow the other on PATH)"
  fi
  return 0
}

# ---- release resolution (D32) and download ---------------------------------

TAG=""
ASSET=""
CHECKSUMS=""

# "latest" resolves via the releases/latest redirect, not the JSON API: the
# API is rate-limited at 60/hr/IP unauthenticated and fails behind CI and
# office NAT — exactly where bootstrap runs.
resolve_tag() {
  if [ -n "$REQ_VERSION" ]; then
    TAG="v$REQ_VERSION"
  else
    # -w emits even when -f fails (0-release repo → "404 <url>", network
    # failure → "000 <url>"; verified live 2026-07-18), so the two failures
    # get distinct messages: a repo with no releases is not a network problem.
    latest_meta=$(fetch -I -o /dev/null -w '%{http_code} %{url_effective}' \
      "https://github.com/$REPO/releases/latest") || true
    latest_code="${latest_meta%% *}"
    latest_url="${latest_meta#* }"
    case "$latest_code" in
      404) die "no releases found for $REPO" \
               "the repo has no published release yet; pass --version vX.Y.Z if you know one" ;;
      2*|3*) ;;
      *) die "could not resolve the latest release of $REPO" \
             "check the network, or pass --version vX.Y.Z" ;;
    esac
    case "$latest_url" in
      */releases/tag/*) TAG="${latest_url##*/releases/tag/}"; TAG="${TAG%%\?*}" ;;
      *) die "no releases found for $REPO" \
             "the repo has no published release yet; pass --version vX.Y.Z if you know one" ;;
    esac
  fi
  ASSET="${BIN}_${TAG}_${OS}_${ARCH}.tar.gz"
  CHECKSUMS="${BIN}_${TAG}_checksums.txt"
}

# The full plan prints at normal level under --dry-run (it is the output the
# flag exists for) and at verbose otherwise.
print_plan() {
  if [ "$DRY_RUN" = 1 ]; then plan_log=log_info; else plan_log=log_detail; fi
  "$plan_log" "bin:      $BIN"
  "$plan_log" "platform: $OS/$ARCH"
  "$plan_log" "release:  $TAG"
  "$plan_log" "asset:    $ASSET"
  "$plan_log" "install:  $INSTALL_DIR/$BIN"
  if [ "$BIN_ONLY" = 0 ]; then
    "$plan_log" "extras:   man pages + completions, when the archive ships them"
  fi
}

WORK_DIR=""

download() {
  WORK_DIR=$(mktemp -d)
  trap 'rm -rf "$WORK_DIR"' EXIT
  base_url="https://github.com/$REPO/releases/download/$TAG"
  log_info "downloading $ASSET..."
  log_detail "from: $base_url/$ASSET"
  fetch -o "$WORK_DIR/$ASSET" "$base_url/$ASSET" \
    || die "download failed: $base_url/$ASSET" \
           "is $TAG a published $REPO release with a $OS/$ARCH build?"
  fetch -o "$WORK_DIR/$CHECKSUMS" "$base_url/$CHECKSUMS" \
    || die "download failed: $base_url/$CHECKSUMS" \
           "every release publishes this file; a missing one is a broken release"
}

# ---- verification (F4, F5) --------------------------------------------------

verify_checksum() {
  # exact-string field match — a regex would let '.' in the asset name wildcard
  want=$(awk -v a="$ASSET" '$2 == a {print $1; exit}' "$WORK_DIR/$CHECKSUMS") || want=""
  if [ -z "$want" ]; then
    die "no checksum for $ASSET in $CHECKSUMS" \
        "the release is malformed; report it at https://github.com/$REPO/issues"
  fi
  case "$SHA_TOOL" in
    sha256sum) got=$(sha256sum "$WORK_DIR/$ASSET" | awk '{print $1}') ;;
    shasum)    got=$(shasum -a 256 "$WORK_DIR/$ASSET" | awk '{print $1}') ;;
  esac
  if [ "$want" != "$got" ]; then
    die "checksum mismatch for $ASSET" \
        "expected $want, got $got — corrupted or truncated download; re-run to retry"
  fi
  log_detail "checksum verified: sha256 $got"
}

# Opportunistic (F5): verify iff cosign is present — requiring it would fail
# bootstrap on exactly the fresh machines bootstrap serves. --require-signature
# hardens it to mandatory. This, not F4, is the authenticity check.
verify_signature() {
  if ! command -v cosign >/dev/null 2>&1; then
    if [ "$REQUIRE_SIG" = 1 ]; then
      die "--require-signature was given but cosign is not installed" \
          "install cosign (https://docs.sigstore.dev), then re-run"
    fi
    log_warn "notice: cosign not found — skipping signature verification (install cosign to verify releases)"
    return 0
  fi
  base_url="https://github.com/$REPO/releases/download/$TAG"
  # cosign v3 signs blobs into a single Sigstore bundle (checksums.txt.sigstore.json)
  # carrying both the signature and the Fulcio certificate; the old detached
  # .sig/.pem pair is gone (release-ci #45). Verifying the new bundle format needs
  # cosign v3+ — an older cosign here degrades like a missing signature (below).
  # 2>/dev/null: this fetch is opportunistic — a release with no bundle is a
  # clean skip below, not an error, so curl's own 404 noise must not leak (the
  # mandatory archive/checksums fetches keep their diagnostics).
  if ! fetch -o "$WORK_DIR/$CHECKSUMS.sigstore.json" "$base_url/$CHECKSUMS.sigstore.json" 2>/dev/null; then
    if [ "$REQUIRE_SIG" = 1 ]; then
      die "--require-signature was given but $TAG publishes no signature"
    fi
    log_warn "notice: no signature published for $TAG — skipping signature verification"
    return 0
  fi
  signer_re=$(printf '%s' "$SIGNER_REPO" | sed 's/\./\\./g')
  if cosign verify-blob \
      --bundle "$WORK_DIR/$CHECKSUMS.sigstore.json" \
      --certificate-identity-regexp "^https://github\\.com/${signer_re}/\\.github/workflows/release\\.yml@refs/tags/v" \
      --certificate-oidc-issuer "https://token.actions.githubusercontent.com" \
      "$WORK_DIR/$CHECKSUMS" >/dev/null 2>&1; then
    log_info "signature verified (signer: $SIGNER_REPO)"
  else
    die "signature verification FAILED for $CHECKSUMS" \
        "the artifact does not carry a valid $SIGNER_REPO signature — do not install it; report at https://github.com/$REPO/issues"
  fi
}

# ---- install (D20) ----------------------------------------------------------

# install_extra SRC DEST — best-effort: the binary is what the install is for,
# so a failed extra is a notice, never an error.
install_extra() {
  dest_dir=$(dirname "$2")
  if mkdir -p "$dest_dir" 2>/dev/null && cp "$1" "$2" 2>/dev/null; then
    log_detail "installed: $2"
  else
    log_warn "notice: could not install $2 (skipped)"
  fi
}

# Man pages and completions install whenever the archive ships them — look,
# don't ask (D20): parity with apt/brew is the point of the fallback path, and
# the archive itself is the source of truth. --bin-only declines at runtime.
install_extras() {
  data_home="${XDG_DATA_HOME:-$HOME/.local/share}"
  config_home="${XDG_CONFIG_HOME:-$HOME/.config}"
  zsh_dir="$data_home/zsh/site-functions"
  installed_zsh=0

  for f in "$WORK_DIR"/man/*; do
    [ -f "$f" ] || continue
    page=$(basename "$f")
    section="${page##*.}"
    case "$section" in
      [0-9]*) install_extra "$f" "$data_home/man/man$(printf '%s' "$section" | cut -c1)/$page" ;;
    esac
  done

  for f in "$WORK_DIR"/completions/*; do
    [ -f "$f" ] || continue
    name=$(basename "$f")
    case "$name" in
      *.bash) install_extra "$f" "$data_home/bash-completion/completions/${name%.bash}" ;;
      _*)     install_extra "$f" "$zsh_dir/$name"; installed_zsh=1 ;;
      *.fish) install_extra "$f" "$config_home/fish/completions/$name" ;;
    esac
  done

  # zsh, unlike bash and fish, has no user completion dir it searches by default.
  if [ "$installed_zsh" = 1 ]; then
    case ":${FPATH:-}:" in
      *":$zsh_dir:"*) ;;
      *)
        log_info "for zsh completion, add to ~/.zshrc (before compinit):"
        log_info "  fpath=($zsh_dir \$fpath)"
        ;;
    esac
  fi
}

install_files() {
  tar -xzf "$WORK_DIR/$ASSET" -C "$WORK_DIR" \
    || die "could not extract $ASSET"
  if [ ! -f "$WORK_DIR/$BIN" ]; then
    die "archive does not contain $BIN at its root" \
        "the release violates the artifact-shape contract; report at https://github.com/$REPO/issues"
  fi
  mkdir -p "$INSTALL_DIR" \
    || die "could not create $INSTALL_DIR" "pass --install-dir for a writable location"
  # Stage inside the destination dir, then rename: the swap is atomic, and an
  # in-use binary is replaced rather than written through.
  staging="$INSTALL_DIR/.$BIN.tmp.$$"
  if ! mv "$WORK_DIR/$BIN" "$staging" || ! chmod +x "$staging"; then
    rm -f "$staging"
    die "could not write to $INSTALL_DIR" "pass --install-dir for a writable location"
  fi
  if ! mv -f "$staging" "$INSTALL_DIR/$BIN"; then
    rm -f "$staging"
    die "could not write to $INSTALL_DIR" "pass --install-dir for a writable location"
  fi
  log_change "installed: $INSTALL_DIR/$BIN ($TAG)"

  if [ "$BIN_ONLY" = 0 ]; then install_extras; fi
}

warn_if_off_path() {
  case ":$PATH:" in
    *":$INSTALL_DIR:"*) ;;
    *)
      log_warn "note: $INSTALL_DIR is not on your PATH; add to your shell rc:"
      log_warn "  export PATH=\"$INSTALL_DIR:\$PATH\""
      ;;
  esac
}

# ---- main -------------------------------------------------------------------

main() {
  parse_args "$@"
  require_vendored_config
  read_environment
  require_tools
  detect_platform
  resolve_install_dir
  presence_check
  resolve_tag
  print_plan
  if [ "$DRY_RUN" = 1 ]; then
    log_info "(dry-run: no changes made)"
    exit 0
  fi
  download
  verify_checksum
  verify_signature
  install_files
  warn_if_off_path
}

main "$@"
