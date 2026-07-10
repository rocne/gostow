#!/usr/bin/env sh
# install.sh — download and install gostow, a single static binary.
#
# One-liner:
#   curl -fsSL https://raw.githubusercontent.com/rocne/gostow/main/install.sh | sh
#
# With args (pass after --):
#   curl -fsSL https://raw.githubusercontent.com/rocne/gostow/main/install.sh | sh -s -- --version v0.2.0
#
# Local usage:
#   ./install.sh [--version v0.2.0] [--os linux|darwin] [--arch amd64|arm64]
#                [--dir path] [--bin-only] [--dry-run]
#
# --version    specific version to install  (default: latest)  e.g. v0.2.0
# --os         override OS detection
# --arch       override architecture detection
# --dir        override install directory   (default: ~/.local/bin)
# --bin-only   install only the binary; skip the man page and completions
# --dry-run    print what would be done, then exit without installing
#
# Requires: curl, and sha256sum or shasum (checksum verification is mandatory).
# cosign, if present, additionally verifies the release signature.
#
# This installs the `gostow` binary. gostow is a drop-in replacement for GNU
# Stow; to make it answer to `stow` too, see "Using it as a drop-in" in the
# README. This script never claims the name `stow`.
set -e

REPO="rocne/gostow"
TOOL="gostow"

# --- defaults ---
VERSION=""
OS=""
ARCH=""
INSTALL_DIR="${INSTALL_DIR:-$HOME/.local/bin}"
BIN_ONLY=0
DRY_RUN=0

# --- parse args ---
while [ $# -gt 0 ]; do
  case "$1" in
    --version)  VERSION="$2"; shift 2 ;;
    --os)       OS="$2";      shift 2 ;;
    --arch)     ARCH="$2";    shift 2 ;;
    --dir)      INSTALL_DIR="$2"; shift 2 ;;
    --bin-only) BIN_ONLY=1; shift ;;
    --dry-run)  DRY_RUN=1; shift ;;
    --help|-h)
      sed -n '2,23p' "$0" | sed 's/^# \?//'
      exit 0
      ;;
    *) printf 'error: unknown argument: %s\n' "$1" >&2; exit 1 ;;
  esac
done

# --- detect OS (unless overridden) ---
if [ -z "$OS" ]; then
  OS=$(uname -s)
  case "$OS" in
    Linux)  OS="linux"  ;;
    Darwin) OS="darwin" ;;
    *) printf 'error: unsupported OS: %s (use --os to override)\n' "$OS" >&2; exit 1 ;;
  esac
fi

# --- detect arch (unless overridden) ---
if [ -z "$ARCH" ]; then
  ARCH=$(uname -m)
  case "$ARCH" in
    x86_64|amd64)  ARCH="amd64" ;;
    arm64|aarch64) ARCH="arm64" ;;
    *) printf 'error: unsupported architecture: %s (use --arch to override)\n' "$ARCH" >&2; exit 1 ;;
  esac
fi

printf 'tool:     %s\n' "$TOOL"
printf 'platform: %s/%s\n' "$OS" "$ARCH"

# --- require curl ---
if ! command -v curl >/dev/null 2>&1; then
  printf 'error: curl not found\n' >&2
  exit 1
fi

# --- resolve version to tag ---
if [ -n "$VERSION" ]; then
  # normalize: strip a leading 'v' then re-add, so both 'v0.2.0' and '0.2.0' work
  VERSION=$(printf '%s' "$VERSION" | sed 's/^v//')
  TAG="v${VERSION}"
else
  TAG=$(curl -fsSL "https://api.github.com/repos/$REPO/releases/latest" \
    | grep '"tag_name"' \
    | sed 's/.*"tag_name": *"\([^"]*\)".*/\1/')

  if [ -z "$TAG" ] || [ "$TAG" = "null" ]; then
    printf 'error: no %s release found in %s\n' "$TOOL" "$REPO" >&2
    exit 1
  fi
fi

printf 'release:  %s\n' "$TAG"

# tag: v0.2.0  →  asset: gostow_v0.2.0_linux_amd64.tar.gz
ASSET_PREFIX="${TOOL}_${TAG}"
ASSET="${ASSET_PREFIX}_${OS}_${ARCH}.tar.gz"
CHECKSUMS="${ASSET_PREFIX}_checksums.txt"
printf 'asset:    %s\n' "$ASSET"

if [ "$DRY_RUN" = "1" ]; then
  printf 'install:  %s/%s\n' "$INSTALL_DIR" "$TOOL"
  [ "$BIN_ONLY" = "0" ] && printf 'extras:   man page + shell completions\n'
  printf '(dry-run: no changes made)\n'
  exit 0
fi

# --- download ---
TMP=$(mktemp -d)
trap 'rm -rf "$TMP"' EXIT

printf 'downloading...\n'
BASE_URL="https://github.com/$REPO/releases/download/$TAG"
curl -fsSL -o "$TMP/$ASSET"     "$BASE_URL/$ASSET"
curl -fsSL -o "$TMP/$CHECKSUMS" "$BASE_URL/$CHECKSUMS"

# --- verify checksum (mandatory) ---
printf 'verifying checksum...\n'
if command -v sha256sum >/dev/null 2>&1; then
  # filter to just this asset's line so a missing sibling file is not an error
  grep " ${ASSET}\$" "$TMP/$CHECKSUMS" | (cd "$TMP" && sha256sum -c -)
elif command -v shasum >/dev/null 2>&1; then
  grep " ${ASSET}\$" "$TMP/$CHECKSUMS" | (cd "$TMP" && shasum -a 256 -c -)
else
  printf 'error: no sha256sum or shasum found — cannot verify download integrity; aborting\n' >&2
  printf '       install coreutils (for sha256sum) or run on a system with shasum, then retry.\n' >&2
  exit 1
fi

# --- verify signature (enforced when cosign is present) ---
SIG="${CHECKSUMS}.sig"
CERT="${CHECKSUMS}.pem"
if command -v cosign >/dev/null 2>&1; then
  if curl -fsSL -o "$TMP/$SIG" "$BASE_URL/$SIG" 2>/dev/null \
    && curl -fsSL -o "$TMP/$CERT" "$BASE_URL/$CERT" 2>/dev/null; then
    printf 'verifying signature...\n'
    if cosign verify-blob \
        --certificate "$TMP/$CERT" \
        --signature   "$TMP/$SIG" \
        --certificate-identity-regexp "^https://github\.com/rocne/release-ci/\.github/workflows/release\.yml@refs/tags/v" \
        --certificate-oidc-issuer "https://token.actions.githubusercontent.com" \
        "$TMP/$CHECKSUMS" >/dev/null 2>&1; then
      printf 'signature verified\n'
    else
      printf 'error: signature verification FAILED for %s — aborting\n' "$CHECKSUMS" >&2
      exit 1
    fi
  else
    printf 'notice: no signature published for %s — skipping signature verification\n' "$TAG" >&2
  fi
else
  printf 'notice: cosign not found — skipping signature verification (install cosign v2+ for full verification)\n' >&2
fi

# --- extract and install the binary ---
tar -xzf "$TMP/$ASSET" -C "$TMP"
mkdir -p "$INSTALL_DIR"
mv "$TMP/$TOOL" "$INSTALL_DIR/$TOOL"
chmod +x "$INSTALL_DIR/$TOOL"
printf 'installed: %s/%s\n' "$INSTALL_DIR" "$TOOL"

# --- best-effort: man page and shell completions ---
# These land in per-user XDG locations, so no root is needed, and any failure is
# a notice rather than an error: the binary is what the install is for.
install_extra() {
  # $1 = source path in the extracted tarball, $2 = destination path
  [ -f "$TMP/$1" ] || return 0
  dest_dir=$(dirname "$2")
  if mkdir -p "$dest_dir" 2>/dev/null && cp "$TMP/$1" "$2" 2>/dev/null; then
    printf 'installed: %s\n' "$2"
  else
    printf 'notice: could not install %s (skipped)\n' "$2" >&2
  fi
}

if [ "$BIN_ONLY" = "0" ]; then
  DATA_HOME="${XDG_DATA_HOME:-$HOME/.local/share}"
  CONFIG_HOME="${XDG_CONFIG_HOME:-$HOME/.config}"

  install_extra "man/gostow.8"        "$DATA_HOME/man/man8/gostow.8"
  install_extra "completions/gostow.bash" "$DATA_HOME/bash-completion/completions/gostow"
  install_extra "completions/_gostow"     "$DATA_HOME/zsh/site-functions/_gostow"
  install_extra "completions/gostow.fish" "$CONFIG_HOME/fish/completions/gostow.fish"

  # zsh, unlike bash and fish, has no user completion dir it searches by default.
  if [ -f "$DATA_HOME/zsh/site-functions/_gostow" ]; then
    case ":$FPATH:" in
      *":$DATA_HOME/zsh/site-functions:"*) ;;
      *)
        printf '\nFor zsh completion, add to your ~/.zshrc (before compinit):\n'
        # $fpath is literal here — it is shell syntax for the user to paste, not
        # a variable for this script to expand.
        # shellcheck disable=SC2016
        printf '  fpath=(%s/zsh/site-functions $fpath)\n' "$DATA_HOME"
        ;;
    esac
  fi
fi

printf '\n'
"$INSTALL_DIR/$TOOL" --version

# --- PATH reminder ---
case ":$PATH:" in
  *":$INSTALL_DIR:"*) ;;
  *)
    printf '\nNote: %s is not in your PATH. Add to your shell rc:\n' "$INSTALL_DIR"
    # $PATH is literal here — it is what the user pastes into their rc.
    # shellcheck disable=SC2016
    printf '  export PATH="%s:$PATH"\n' "$INSTALL_DIR"
    ;;
esac
