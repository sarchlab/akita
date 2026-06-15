#!/usr/bin/env bash
#
# build_oracles.sh — fetch and build DRAMSim3 and Ramulator2 at the pinned
# commits in COMMITS.txt. Idempotent: re-running rebuilds in place.
#
# Usage:   ./build_oracles.sh [WORKDIR]
# Default WORKDIR: ./.oracles (gitignored). Produces:
#   $WORKDIR/DRAMsim3/build/dramsim3main
#   $WORKDIR/ramulator2/build/ramulator2
#
# Requires: git, cmake (>=3.14), a C++20 compiler (g++>=11 / clang>=14), make.
# Needs network access to clone the two upstream repos and (Ramulator2) fetch
# its build-time dependencies.
set -euo pipefail

HERE="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
# shellcheck source=/dev/null
source "$HERE/COMMITS.txt"

WORKDIR="${1:-$HERE/.oracles}"
mkdir -p "$WORKDIR"
JOBS="$(nproc 2>/dev/null || echo 4)"

clone_pinned() {
  local repo="$1" commit="$2" dir="$3"
  if [ ! -d "$dir/.git" ]; then
    git clone "$repo" "$dir"
  fi
  git -C "$dir" fetch --depth 1 origin "$commit" || git -C "$dir" fetch origin
  git -C "$dir" checkout -q "$commit"
}

echo "==> DRAMSim3 @ $DRAMSIM3_COMMIT"
clone_pinned "$DRAMSIM3_REPO" "$DRAMSIM3_COMMIT" "$WORKDIR/DRAMsim3"
cmake -S "$WORKDIR/DRAMsim3" -B "$WORKDIR/DRAMsim3/build" -DCMAKE_BUILD_TYPE=Release >/dev/null
cmake --build "$WORKDIR/DRAMsim3/build" -j "$JOBS" >/dev/null
echo "    built $WORKDIR/DRAMsim3/build/dramsim3main"

echo "==> Ramulator2 @ $RAMULATOR2_COMMIT"
clone_pinned "$RAMULATOR2_REPO" "$RAMULATOR2_COMMIT" "$WORKDIR/ramulator2"
cmake -S "$WORKDIR/ramulator2" -B "$WORKDIR/ramulator2/build" -DCMAKE_BUILD_TYPE=Release >/dev/null
cmake --build "$WORKDIR/ramulator2/build" -j "$JOBS" >/dev/null
echo "    built $WORKDIR/ramulator2/build/ramulator2"

echo "==> oracles ready under $WORKDIR"
