#!/usr/bin/env bash

set -euo pipefail

GOVULNCHECK_VERSION="v1.3.0"

root_dir="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
report_dir="${DEPENDENCY_SECURITY_REPORT_DIR:-}"
if [[ -z "${report_dir}" ]]; then
  report_dir="$(mktemp -d "${TMPDIR:-/tmp}/akita-dependency-security.XXXXXX")"
else
  mkdir -p "${report_dir}"
fi

log_dir="${report_dir}/logs"
bin_dir="${report_dir}/bin"
mkdir -p "${log_dir}" "${bin_dir}"

cd "${root_dir}"

run_logged() {
  local name="$1"
  shift
  local log_file="${log_dir}/${name}.log"

  {
    printf '+'
    printf ' %q' "$@"
    printf '\n'
  } | tee "${log_file}.cmd"

  if "$@" >"${log_file}" 2>&1; then
    cat "${log_file}"
    return 0
  fi

  local status=$?
  cat "${log_file}"
  return "${status}"
}

printf 'Dependency security validation report: %s\n' "${report_dir}"

run_logged go_version go version
run_logged go_env go env GOVERSION GOTOOLCHAIN GOMOD
run_logged module_list go list -mod=readonly -m all
run_logged module_graph go mod graph
run_logged go_mod_tidy_diff go mod tidy -diff
run_logged go_test go test ./...
run_logged git_diff_check git diff --check

run_logged govulncheck_install env GOBIN="${bin_dir}" go install "golang.org/x/vuln/cmd/govulncheck@${GOVULNCHECK_VERSION}"
run_logged govulncheck_version "${bin_dir}/govulncheck" -version
run_logged govulncheck_test "${bin_dir}/govulncheck" -test ./...

printf 'Dependency security validation completed successfully. Logs: %s\n' "${log_dir}"
