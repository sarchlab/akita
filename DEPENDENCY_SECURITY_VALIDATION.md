# Dependency Security Validation

This is the maintainer path for rerunning Akita dependency and Go toolchain security validation after a future GitHub/Dependabot alert, Go advisory, or dependency bump. It is intentionally local/manual evidence collection rather than a new required CI job.

## Scope and prerequisites

- Run from a clean checkout of the branch being triaged.
- Use the checked-in Go baseline: `go.mod:51-53` pins `go 1.26.0` with `toolchain go1.26.2`, and `TOOLCHAIN_VERSIONS.md:5-12` records the same security-remediated toolchain baseline.
- Allow network access for module metadata, module downloads if the cache is cold, the Go vulnerability database used by govulncheck, and the npm advisory registry used by frontend audits.
- Frontend dependency-security validation is in scope for the checked-in `daisen/static` and `daisen2/static` packages.
- Do not treat this path as an MGPUSim migration implementation; it only validates the current Akita repository's dependency and toolchain evidence.

## One-command validation

Run the version-controlled script:

```bash
./run_dependency_security_validation.sh
```

By default the script writes logs under a temporary `akita-dependency-security.*` directory and prints the exact path. To keep logs in a chosen location, set `DEPENDENCY_SECURITY_REPORT_DIR`:

```bash
DEPENDENCY_SECURITY_REPORT_DIR=/tmp/akita-security-report ./run_dependency_security_validation.sh
```

When `DEPENDENCY_SECURITY_REPORT_DIR` is set, the script creates it if needed and resolves it to a physical, canonical path before deriving `logs/` and the local `GOBIN` directory (`run_dependency_security_validation.sh:10-31`). If the report directory cannot be created or resolved, the script rejects it and exits non-zero before running validation commands.

The script installs a pinned local `govulncheck` binary in the canonical report directory (`golang.org/x/vuln/cmd/govulncheck@v1.3.0`) before running the scan. It does not modify the repository or rely on an unpinned tool already on `PATH`.

Each required command is wrapped by the failure-safe logger in `run_dependency_security_validation.sh:35-110`. On the first command failure, including a frontend npm audit failure, the script prints that command's captured output, reports the failing check name, exits non-zero, and does not print the final `Dependency security validation completed successfully` message.

## Checks performed

The script records the Go version and then runs these repository checks:

```bash
go list -mod=readonly -m all
go mod graph
go mod tidy -diff
go test ./...
git diff --check
govulncheck -test ./...
(cd daisen/static && npm audit --audit-level=high --omit=optional)
(cd daisen2/static && npm audit --audit-level=high --omit=optional)
```

What each check contributes:

- `go list -mod=readonly -m all` records the selected module versions without allowing implicit edits to `go.mod` or `go.sum`.
- `go mod graph` records the full module graph used to interpret transitive dependency alerts.
- `go mod tidy -diff` confirms that module metadata is reproducible and already tidy.
- `go test ./...` preserves the baseline repository test signal while dependency changes are triaged.
- `git diff --check` catches whitespace/conflict-marker issues before evidence is reported.
- `govulncheck -test ./...` evaluates reachable vulnerable symbols in packages and tests using the pinned local govulncheck binary.
- `cd daisen/static && npm audit --audit-level=high --omit=optional` and `cd daisen2/static && npm audit --audit-level=high --omit=optional` make checked-in frontend package audit findings visible and fail validation for high-or-worse non-optional npm advisories. The audits also report lower-severity findings in their output, but the validation threshold is intentionally high to match the maintained gate.

If the script is unavailable, install and run the same govulncheck version locally before executing the manual sequence above, then run both frontend audits explicitly:

```bash
GOBIN="$(mktemp -d)/bin" go install golang.org/x/vuln/cmd/govulncheck@v1.3.0
"${GOBIN}/govulncheck" -test ./...
(cd daisen/static && npm audit --audit-level=high --omit=optional)
(cd daisen2/static && npm audit --audit-level=high --omit=optional)
```

## Interpreting local evidence versus GitHub/Dependabot notices

GitHub and Dependabot notices are asynchronous, default-branch-oriented signals. They may lag behind a branch update, refer to an advisory payload that is not visible locally, or continue to show a default-branch dependency state after a security branch has already updated `go.mod` or the Go toolchain. When an exact alert payload is unavailable, do not claim the notice is false solely because local output is clean.

Use the local report to make the comparison explicit:

1. Record the branch, commit SHA, `go version`, pinned govulncheck version, `go list -mod=readonly -m all`, `go mod graph`, and frontend `npm audit --audit-level=high --omit=optional` logs.
2. Compare the alerted module, Go toolchain, or npm package version with the selected versions in the local module list, checked-in frontend lockfiles, and checked-in toolchain baseline.
3. If `govulncheck -test ./...` reports a reachable vulnerability, update the affected dependency or toolchain and rerun the full script.
4. If either frontend npm audit reports a high-or-worse non-optional vulnerability, update `package-lock.json`/`package.json` where safe and rerun both exact frontend audit commands. If a finding cannot be safely remediated, document the exact residual risk and scope in version-controlled files and keep the validation gate visibly failing or explicitly gated rather than silently bypassing it.
5. If local govulncheck and frontend audits are clean but GitHub/Dependabot still reports a default-branch alert, report the clean local evidence, note the possible asynchronous/default-branch lag, and recheck after the default branch has the same dependency state.
6. If the alert only identifies a vulnerable Go module version but govulncheck reports no reachable symbols, keep the module graph evidence with the triage note so maintainers can decide whether policy still requires an update.

## Why this is local/manual instead of CI

The existing CI workflow keeps the main build, lint, and test signal focused on deterministic repository readiness (`.github/workflows/akita_test.yml:41-65`). govulncheck depends on an external vulnerability database whose contents change asynchronously, and GitHub/Dependabot alerts are generated for default-branch state outside the timing of a local branch test. Running this path manually after an alert gives maintainers reproducible, version-controlled commands and saved logs without making routine CI fail because an external advisory feed changed between otherwise identical commits.
