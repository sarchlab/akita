#!/usr/bin/env bash

set -euo pipefail

GOVULNCHECK_VERSION="v1.3.0"

root_dir="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
requested_report_dir="${DEPENDENCY_SECURITY_REPORT_DIR:-}"

prepare_report_dir() {
  local requested="$1"

  if [[ -z "${requested}" ]]; then
    requested="$(mktemp -d "${TMPDIR:-/tmp}/akita-dependency-security.XXXXXX")" || return
  else
    mkdir -p -- "${requested}" || return
  fi

  cd -P -- "${requested}" || return
  pwd -P
}

if ! report_dir="$(prepare_report_dir "${requested_report_dir}")"; then
  printf 'Dependency security validation failed: unable to prepare report directory %q\n' \
    "${requested_report_dir:-<temporary>}" >&2
  exit 1
fi

log_dir="${report_dir}/logs"
bin_dir="${report_dir}/bin"
mkdir -p -- "${log_dir}" "${bin_dir}"

cd "${root_dir}"

write_command_log() {
  local log_file="$1"
  shift

  {
    printf '+'
    printf ' %q' "$@"
    printf '\n'
  } | tee "${log_file}.cmd"
}

run_logged() {
  local name="$1"
  shift
  local log_file="${log_dir}/${name}.log"

  write_command_log "${log_file}" "$@"

  if "$@" >"${log_file}" 2>&1; then
    cat "${log_file}"
    return 0
  else
    local status=$?
    cat "${log_file}"
    return "${status}"
  fi
}

run_logged_in() {
  local name="$1"
  local work_dir="$2"
  shift 2
  local log_file="${log_dir}/${name}.log"

  write_command_log "${log_file}" cd "${work_dir}" "&&" "$@"

  if (cd "${work_dir}" && "$@") >"${log_file}" 2>&1; then
    cat "${log_file}"
    return 0
  else
    local status=$?
    cat "${log_file}"
    return "${status}"
  fi
}

run_required() {
  local name="$1"
  shift
  local status

  if run_logged "${name}" "$@"; then
    return 0
  else
    status=$?
    printf 'Dependency security validation failed during %s. Logs: %s\n' \
      "${name}" "${log_dir}" >&2
    return "${status}"
  fi
}

run_required_in() {
  local name="$1"
  local work_dir="$2"
  shift 2
  local status

  if run_logged_in "${name}" "${work_dir}" "$@"; then
    return 0
  else
    status=$?
    printf 'Dependency security validation failed during %s. Logs: %s\n' \
      "${name}" "${log_dir}" >&2
    return "${status}"
  fi
}

printf 'Dependency security validation report: %s\n' "${report_dir}"

run_required go_version go version
run_required go_env go env GOVERSION GOTOOLCHAIN GOMOD
run_required module_list go list -mod=readonly -m all
run_required module_graph go mod graph
run_required go_mod_tidy_diff go mod tidy -diff
run_required go_test go test ./...
run_required git_diff_check git diff --check

run_required govulncheck_install \
  env GOBIN="${bin_dir}" go install "golang.org/x/vuln/cmd/govulncheck@${GOVULNCHECK_VERSION}"
run_required govulncheck_version "${bin_dir}/govulncheck" -version
run_required govulncheck_test "${bin_dir}/govulncheck" -test ./...

run_required_in daisen_static_npm_audit daisen/static \
  npm audit --audit-level=high --omit=optional
run_required_in daisen2_static_npm_audit daisen2/static \
  npm audit --audit-level=high --omit=optional

printf 'Dependency security validation completed successfully. Logs: %s\n' "${log_dir}"
