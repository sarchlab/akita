# Toolchain Version Lock

This document describes the toolchain versions and requirements that are checked into
this repository or configured in GitHub Actions.

## Go Toolchain

- **Go language version**: 1.26.0
- **Go toolchain version**: go1.26.2
- **Rationale**: Keep the module language version, local toolchain pin, and CI
  toolchain aligned with the current security-remediated baseline.
- **Configuration**:
  - `go.mod`: `go 1.26.0` and `toolchain go1.26.2`
  - `.github/workflows/akita_test.yml`: `go-version: 1.26.2`

## Go Tools

- **mockgen**: v0.6.0 (was: @latest)
  - Used for generating mock implementations for testing
  - Locked in `run_before_merge.sh` and `.github/workflows/akita_test.yml`

- **ginkgo**: v2.25.1
  - BDD testing framework
  - Locked in `run_before_merge.sh` and `.github/workflows/akita_test.yml`

- **golangci-lint**: v2.9.0
  - Go linter aggregator
  - Locked in `run_before_merge.sh` and `.github/workflows/akita_test.yml`
  - The checked-in `.golangci.yml` keeps the established linter set stable
    across the v2.9.0 compatibility update.

## Node.js Toolchain

- **Node.js Version**: 18.20.7
- **npm Version**: >=10.0.0
- **Configuration**:
  - `daisen/static/.nvmrc`: `18.20.7`
  - `daisen/static/package.json` and `daisen2/static/package.json`: `engines.node`
    is `18.20.7` and `engines.npm` is `>=10.0.0`
  - `.github/workflows/akita_test.yml`: `node-version: 18.20.7` for the
    `daisen/static` and `daisen2/static` build jobs
  - No `.nvmrc` is currently checked in for `daisen2/static`

## Python Toolchain

- **Python Version**: no exact Python version is currently pinned in the repository
  or GitHub Actions.
- **Configuration**:
  - `.github/workflows/akita_test.yml` ensures `python3` is available before
    running the acceptance tests.
  - The workflow does not use `actions/setup-python` and does not configure a
    `python-version` value.

## Verification

To verify all checked-in toolchain settings:

```bash
# Go toolchain settings
grep -nE '^(go|toolchain) ' go.mod
# Should show: go 1.26.0 and toolchain go1.26.2

# GitHub Actions Go and Node settings
grep -nE 'go-version|node-version' .github/workflows/akita_test.yml
# Should show: go-version: 1.26.2 and node-version: 18.20.7

# Node.js version lock and package engine requirements
cat daisen/static/.nvmrc
grep -nE '"engines"|"node"|"npm"' daisen/static/package.json daisen2/static/package.json
# daisen/static/.nvmrc should contain 18.20.7; both package files should define
# Node/npm engines.

# Python workflow behavior
grep -nE 'setup-python|python-version|python3' .github/workflows/akita_test.yml
# Should show python3 checks/runs, and no setup-python or python-version entries.
```

## Updating Locked Versions

When updating to new versions, follow these steps:

1. Update `go.mod` with the new Go language version and toolchain when applicable.
2. Update all Go and Node version pins in `.github/workflows/akita_test.yml`.
3. Update `run_before_merge.sh` if Go tool installation versions change.
4. Update `daisen/static/.nvmrc` if the Node version changes.
5. Update the `engines` fields in `daisen/static/package.json` and
   `daisen2/static/package.json`.
6. Add or update Python version pins only if the repository or workflow starts
   enforcing one.
7. Run the relevant test suite to verify compatibility.
8. Update this document with the new checked-in facts.
