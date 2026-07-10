#!/usr/bin/env bash
# install-stow-oracle.sh — build and install the pinned GNU Stow conformance oracle.
#
# gostow's spec IS real stow's behaviour at an exact version (docs/SPEC.md §1), so
# the differential tests must run against that exact version. Distro packages are
# not good enough: Ubuntu 24.04 ships stow 2.3.1, which would silently redefine
# the spec. So we build the pinned tarball from source and verify its checksum.
#
# Usage:
#   sudo bash test/install-stow-oracle.sh              # install to /usr/local
#   PREFIX=~/.local bash test/install-stow-oracle.sh   # install somewhere else
set -euo pipefail

STOW_VERSION="${STOW_VERSION:-2.4.1}"
STOW_SHA256="${STOW_SHA256:-2a671e75fc207303bfe86a9a7223169c7669df0a8108ebdf1a7fe8cd2b88780b}"
PREFIX="${PREFIX:-/usr/local}"

TARBALL="stow-${STOW_VERSION}.tar.gz"

# Mirrors, tried in order. ftp.gnu.org is the canonical home and is also a single
# point of failure: it went unreachable mid-CI on 2026-07-10 and took the whole
# conformance job with it, 134 seconds after the connect began.
#
# Falling back is safe *because* the tarball is checksum-pinned below. A mirror
# cannot substitute content without failing sha256sum, so the trust root is the
# hash in this file, not the host that served the bytes. ftpmirror.gnu.org is
# GNU's own redirector to a nearby mirror; the third is a long-standing one.
MIRRORS=(
    "https://ftp.gnu.org/gnu/stow/${TARBALL}"
    "https://ftpmirror.gnu.org/stow/${TARBALL}"
    "https://mirrors.kernel.org/gnu/stow/${TARBALL}"
)

workdir="$(mktemp -d)"
trap 'rm -rf "$workdir"' EXIT
cd "$workdir"

fetched=""
for url in "${MIRRORS[@]}"; do
    echo "==> fetching ${url}"
    # --connect-timeout so an unreachable host fails in seconds, not minutes;
    # --location because ftpmirror.gnu.org answers with a redirect.
    if curl -fsSL --location --connect-timeout 20 --max-time 300 -o "$TARBALL" "$url"; then
        fetched="$url"
        break
    fi
    echo "    unreachable; trying the next mirror"
done

if [ -z "$fetched" ]; then
    echo "error: could not fetch ${TARBALL} from any of ${#MIRRORS[@]} mirrors" >&2
    exit 1
fi

echo "==> verifying sha256"
echo "${STOW_SHA256}  ${TARBALL}" | sha256sum -c -

echo "==> building"
tar xzf "$TARBALL"
cd "stow-${STOW_VERSION}"
./configure --prefix="$PREFIX" >/dev/null
make >/dev/null
make install >/dev/null

echo "==> installed: $("${PREFIX}/bin/stow" --version)"

# Fail loudly rather than let a mismatched oracle quietly rewrite the spec.
got="$("${PREFIX}/bin/stow" --version | sed -n 's/.*version \([0-9.]*\).*/\1/p')"
if [ "$got" != "$STOW_VERSION" ]; then
  echo "error: built stow reports '$got', expected '$STOW_VERSION'" >&2
  exit 1
fi
