#!/usr/bin/env bash
# Smoke-test installing gostow from the hosted Cloudsmith repo (rocne/releases):
# register the repo via its setup script, install the gostow package by name,
# and confirm the installed `gostow` binary's --version matches the release.
#
# Note this asserts gostow's OWN version, not the GNU Stow version it clones —
# `gostow --version` prints "gostow 0.1.0 (GNU Stow 2.4.1 compatible)". See
# docs/SPEC.md §4.4; this is gostow's one intentional break with stow's output.
#
# Run inside a clean Debian/Ubuntu (MGR=apt) or Fedora/RHEL (MGR=dnf) container.
# Always invoke with bash (uses `set -o pipefail`); the container's default
# /bin/sh may be dash, which does not support it.
#
# Usage:
#   MGR=apt VERSION=v0.1.0 bash test/run-repo-install-smoke.sh
#   MGR=dnf VERSION=v0.1.0 bash test/run-repo-install-smoke.sh
set -euxo pipefail

: "${MGR:?set MGR=apt|dnf}"
: "${VERSION:?set VERSION=vX.Y.Z}"
VER="${VERSION#v}"

case "$MGR" in
  apt)
    export DEBIAN_FRONTEND=noninteractive
    apt-get update
    apt-get install -y curl sudo
    curl -1sLf 'https://dl.cloudsmith.io/public/rocne/releases/setup.deb.sh' | bash
    # The repo index can lag a few seconds behind the push; retry.
    for i in 1 2 3 4 5; do
      { apt-get update && apt-get install -y gostow && break; } \
        || { echo "retry $i: gostow not in index yet"; sleep 15; }
    done
    ;;
  dnf)
    dnf install -y curl sudo
    curl -1sLf 'https://dl.cloudsmith.io/public/rocne/releases/setup.rpm.sh' | bash
    for i in 1 2 3 4 5; do
      dnf install -y gostow && break \
        || { echo "retry $i: gostow not in index yet"; sleep 15; }
    done
    ;;
  *)
    echo "unknown MGR: $MGR (expected apt or dnf)" >&2
    exit 2
    ;;
esac

command -v gostow
gostow --version
gostow --version | grep -F "$VER"
