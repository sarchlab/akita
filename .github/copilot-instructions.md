# Akita Computer Architecture Simulation Framework

Akita is a Go-based computer architecture simulation framework with web visualization tools. Always reference these instructions first and fallback to search or bash commands only when you encounter unexpected information that does not match the info here.

## Working Effectively

### Bootstrap and Build the Repository
Run these commands in sequence to set up a working development environment:

```bash
# 1. Set up Go PATH (required for tools)
export PATH=$PATH:$(go env GOPATH)/bin

# 2. Install required tools
go install go.uber.org/mock/mockgen@latest
go install github.com/onsi/ginkgo/v2/ginkgo@v2.25.1

# 3. Generate mock files
go generate ./...
# Takes ~30 seconds. NEVER CANCEL. Set timeout to 60+ seconds.

# 4. Build all Go packages
go build ./...
# Takes ~60 seconds including dependency downloads. NEVER CANCEL. Set timeout to 120+ seconds.

# 5. Install golangci-lint for linting
curl -sfL https://raw.githubusercontent.com/golangci/golangci-lint/master/install.sh| sh -s -- -b $(go env GOPATH)/bin v2.4.0

# 6. Build web components (required for full functionality)
cd monitoring/web && npm install && npm run build
# Takes ~12 seconds. Set timeout to 60+ seconds.

cd ../../daisen/static && npm install && npm run build  
# Takes ~18 seconds. Set timeout to 60+ seconds.

cd ../.. # return to root
```

### Run Tests
- **Unit tests**: `ginkgo -r` 
  - Takes ~22 seconds for all 36 test suites. NEVER CANCEL. Set timeout to 60+ seconds.
- **NoC acceptance tests**: `cd noc/acceptance && python3 acceptance_test.py`
  - Takes ~3 minutes. NEVER CANCEL. Set timeout to 300+ seconds.
- **Mem acceptance tests**: `cd mem && python3 acceptance_test.py` 
  - Takes ~2 minutes. NEVER CANCEL. Set timeout to 180+ seconds.

### Linting and Code Quality
- **Lint**: `golangci-lint run ./...`
  - Takes ~5 seconds. Set timeout to 60+ seconds.
  - NOTE: Currently shows 35 style issues (mainly `noctx`, `noinlineerr`, `wsl_v5` warnings). These are not blocking.

### Build Individual Components
- **Daisen visualization server**: `cd daisen && go build` (~1 second)
- **Example projects**: `cd examples/ping && go test -v` (demonstrates working simulator)

## Validation Scenarios

### Always Test After Making Changes
1. **Build validation**: Run `go build ./...` to ensure all packages compile
2. **Unit test validation**: Run `ginkgo -r` to ensure tests pass
3. **Lint validation**: Run `golangci-lint run ./...` to check code style
4. **Example validation**: Run `cd examples/ping && go test -v` to verify basic simulator functionality

### Web Component Validation
- **Monitoring tool**: After building, check that `monitoring/web/dist/` contains built assets
- **Daisen tool**: After building, check that `daisen/static/dist/` contains built assets
- **Daisen server**: Run `cd daisen && ./daisen -help` to verify server binary works

### Manual Testing Scenarios
- **Basic simulator**: Run ping example via `cd examples/ping && go test -v` 
- **Acceptance tests**: Run either NoC or Mem acceptance tests to verify end-to-end functionality
- **Daisen visualization**: Build Daisen server and verify it compiles without errors

## Critical Timing and Timeout Requirements

**NEVER CANCEL builds or long-running commands. Set these minimum timeouts:**
- `go build ./...`: 120+ seconds (typically 8-60s depending on downloads)
- `go generate ./...`: 60+ seconds (typically 28-30s)
- `ginkgo -r`: 60+ seconds (typically 14-22s) 
- `golangci-lint run ./...`: 60+ seconds (typically 5s)
- `npm install && npm run build`: 60+ seconds per web component
- NoC acceptance tests: 300+ seconds (typically 3 minutes)
- Mem acceptance tests: 180+ seconds (typically 2 minutes)

## Repository Structure and Key Projects

### Core Go Modules
- **`sim/`**: Core simulation engine with event scheduling and parallel/serial execution
- **`mem/`**: Memory system components (caches, DRAM, virtual memory, MMU, TLB)
- **`noc/`**: Network-on-Chip components (messaging, switching, arbitration, mesh networks)
- **`tracing/`**: Simulation tracing and profiling infrastructure
- **`monitoring/`**: Real-time monitoring and web dashboard (AkitaRTM)
- **`pipelining/`**: Pipeline simulation components
- **`analysis/`**: Performance analysis tools

### Visualization Tools  
- **`daisen/`**: Main visualization tool with Go backend and TypeScript frontend
- **`monitoring/web/`**: AkitaRTM monitoring dashboard with TypeScript frontend

### Examples and Documentation
- **`examples/`**: Sample simulators including ping, tickingping, and cell_split
- **`doc/`**: Documentation including testing guidelines, Go modules, and component systems

## Common Development Tasks

### Adding New Components
1. Follow dependency injection pattern - components depend on interfaces, not concrete types
2. Create builder pattern for component configuration (see existing examples)
3. Add Ginkgo unit tests with mocking using gomock
4. Ensure "With" functions return builder pointer for method chaining

### Testing Strategy
- **Unit tests**: Test individual component logic using mocks (Ginkgo + Gomega)
- **Integration tests**: Test component interactions without mocks
- **Acceptance tests**: End-to-end scenarios testing complete simulator behavior

### Pre-commit Validation
Always run these commands before committing changes:
```bash
export PATH=$PATH:$(go env GOPATH)/bin
go generate ./...
go build ./...
golangci-lint run ./...
ginkgo -r
```

## Troubleshooting Common Issues

### PATH Issues with Tools
If you get "executable file not found" errors:
```bash
export PATH=$PATH:$(go env GOPATH)/bin
```

### Mock Generation Failures  
If `go generate` fails, ensure mockgen is installed:
```bash
go install go.uber.org/mock/mockgen@latest
```

### Web Build Failures
For npm build issues, try:
```bash
cd monitoring/web && rm -rf node_modules && npm install && npm run build
cd ../../daisen/static && rm -rf node_modules && npm install && npm run build
```

### Ginkgo Version Mismatch Warning
When running tests, you may see a version mismatch warning:
```
Ginkgo detected a version mismatch between the Ginkgo CLI and the version of Ginkgo imported by your packages
```
This is harmless and can be ignored. The tests will run correctly despite this warning. The project uses Ginkgo v2.25.1 in go.mod but the CLI installs the same version.

## Key Files and Configuration

### Go Configuration
- **`go.mod`**: Go 1.25 with standard simulation dependencies
- **`.golangci.yml`**: Linting configuration (relaxed rules for simulation code)
- **`run_before_merge.sh`**: Complete validation script (useful reference)

### Web Configuration  
- **`monitoring/web/package.json`**: AkitaRTM dependencies and build scripts
- **`daisen/static/package.json`**: Daisen frontend dependencies and build scripts

### GitHub Actions
- **`.github/workflows/akita_test.yml`**: CI pipeline covering all components
- Includes compilation, linting, unit tests, and acceptance tests
- Uses GitHub Large Runners for test execution

## Documentation and Resources

- **Main documentation**: https://akitasim.dev/docs/akita/
- **Testing philosophy**: See `doc/testing.md` for TDD guidelines
- **Go modules**: See `doc/gomod.md` for dependency management
- **Component system**: See `doc/component_system.md` for architecture
- **Daisen demo**: Video available in `daisen/README.md`