#!/usr/bin/env bash

set -Eeuo pipefail

MOCKGEN_VERSION="v0.6.0"
GINKGO_VERSION="v2.25.1"
GOLANGCI_LINT_VERSION="v2.9.0"

root_dir="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
temp_dir="$(mktemp -d "${TMPDIR:-/tmp}/akita-run-before-merge.XXXXXX")"
bin_dir="${temp_dir}/bin"
lint_cache_dir="${temp_dir}/golangci-lint-cache"

cleanup() {
  rm -rf -- "${temp_dir}"
}
trap cleanup EXIT

cd "${root_dir}"
mkdir -p -- "${bin_dir}" "${lint_cache_dir}"

print_command() {
  printf '+'
  printf ' %q' "$@"
  printf '\n'
}

run() {
  print_command "$@"
  "$@"
}

verify_go_mod_sum_clean() {
  local phase="$1"

  if ! git diff --quiet -- go.mod go.sum; then
    printf 'run_before_merge.sh failed: go.mod/go.sum changed during %s.\n' "${phase}" >&2
    git diff --name-only -- go.mod go.sum >&2 || true
    return 1
  fi

  if ! git diff --cached --quiet -- go.mod go.sum; then
    printf 'run_before_merge.sh failed: staged go.mod/go.sum changes present during %s.\n' "${phase}" >&2
    git diff --cached --name-only -- go.mod go.sum >&2 || true
    return 1
  fi
}

verify_tracked_clean() {
  local phase="$1"

  if ! git diff --quiet --; then
    printf 'run_before_merge.sh failed: tracked files changed during %s.\n' "${phase}" >&2
    git diff --name-only -- >&2 || true
    return 1
  fi

  if ! git diff --cached --quiet --; then
    printf 'run_before_merge.sh failed: staged tracked-file changes present during %s.\n' "${phase}" >&2
    git diff --cached --name-only -- >&2 || true
    return 1
  fi
}

verify_go_mod_sum_clean "startup"
verify_tracked_clean "startup"

printf 'Running local Akita Go build/lint/test gate.\n'

run go version
run go env GOVERSION GOTOOLCHAIN GOMOD
run go list -mod=readonly -m all
run go mod tidy -diff

run env GOBIN="${bin_dir}" go install "go.uber.org/mock/mockgen@${MOCKGEN_VERSION}"
run env GOBIN="${bin_dir}" go install "github.com/golangci/golangci-lint/v2/cmd/golangci-lint@${GOLANGCI_LINT_VERSION}"
run env GOBIN="${bin_dir}" go install "github.com/onsi/ginkgo/v2/ginkgo@${GINKGO_VERSION}"

run env PATH="${bin_dir}:${PATH}" go generate -mod=readonly ./...
run go build -mod=readonly ./...
run env GOLANGCI_LINT_CACHE="${lint_cache_dir}" \
  "${bin_dir}/golangci-lint" run --timeout=10m --modules-download-mode=readonly ./...
run "${bin_dir}/ginkgo" -r --mod=readonly

verify_go_mod_sum_clean "validation"
verify_tracked_clean "validation"

printf 'Local Akita Go build/lint/test gate completed successfully.\n'
