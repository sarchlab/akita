# Toolchain Version Lock

This document describes the locked versions of all external tools used in the Akita project.

## Go Toolchain

- **Go language version**: 1.26.0
- **Go toolchain version**: go1.26.2
- **Rationale**: Keep the module language version, local toolchain pin, and CI toolchain aligned with the current security-remediated baseline.
- **Configuration**:
  - `go.mod`: `go 1.26.0` and `toolchain go1.26.2`
  - GitHub Actions: `go-version: 1.26.2`

## Go Tools

- **mockgen**: v0.6.0 (was: @latest)
  - Used for generating mock implementations for testing
  - Locked in `run_before_merge.sh` and `.github/workflows/akita_test.yml`

- **ginkgo**: v2.25.1
  - BDD testing framework
  - Already locked in `run_before_merge.sh` and `.github/workflows/akita_test.yml`

- **golangci-lint**: v2.4.0
  - Go linter aggregator
  - Already locked in `run_before_merge.sh` and `.github/workflows/akita_test.yml`

## Node.js Toolchain

- **Node.js Version**: 18.20.7
- **npm Version**: >=10.0.0
- **Configuration**:
  - `.nvmrc` files in `monitoring/web/` and `daisen/static/`
  - `package.json` engines field in both directories
  - GitHub Actions: `node-version: 18.20.7`

## Python Toolchain

- **Python Version**: 3.10.15
- **Configuration**:
  - GitHub Actions: `python-version: "3.10.15"`

## Verification

To verify all tools are correctly locked:

```bash
# Go toolchain version
go version
# Should output: go version go1.26.2 <os>/<arch>

# Node.js version (in monitoring/web or daisen/static)
node --version
# Should output: v18.20.7

# Python version
python --version
# Should output: Python 3.10.15
```

## Updating Locked Versions

When updating to new versions, follow these steps:

1. Update `go.mod` with new Go version and toolchain
2. Update all occurrences in `.github/workflows/akita_test.yml`
3. Update `run_before_merge.sh` if applicable
4. Update `.nvmrc` files for Node.js
5. Update `package.json` engines fields
6. Run full test suite to verify compatibility
7. Update this document with the new versions
