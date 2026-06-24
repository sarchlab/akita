module github.com/sarchlab/akita/v5

require (
	github.com/glebarez/go-sqlite v1.22.0
	github.com/google/pprof v0.0.0-20250820193118-f64d9cf942d6
	github.com/mattn/go-sqlite3 v1.14.24
	github.com/onsi/ginkgo/v2 v2.25.1
	github.com/onsi/gomega v1.38.1
	github.com/pkg/browser v0.0.0-20240102092130-5ac0b6a4141c
	github.com/rs/xid v1.6.0
	github.com/shirou/gopsutil v3.21.11+incompatible
	github.com/spf13/cobra v1.9.1
	github.com/stretchr/testify v1.11.0
	github.com/syifan/goseth v0.1.2
	github.com/tebeka/atexit v0.3.0
	go.uber.org/mock v0.6.0
	golang.org/x/tools v0.39.0
	modernc.org/sqlite v1.28.0
)

require (
	github.com/Masterminds/semver/v3 v3.4.0 // indirect
	github.com/davecgh/go-spew v1.1.1 // indirect
	github.com/dustin/go-humanize v1.0.1 // indirect
	github.com/go-logr/logr v1.4.3 // indirect
	github.com/go-ole/go-ole v1.3.0 // indirect
	github.com/go-task/slim-sprig/v3 v3.0.0 // indirect
	github.com/google/go-cmp v0.7.0 // indirect
	github.com/google/uuid v1.5.0 // indirect
	github.com/inconshreveable/mousetrap v1.1.0 // indirect
	github.com/kballard/go-shellquote v0.0.0-20180428030007-95032a82bc51 // indirect
	github.com/mattn/go-isatty v0.0.20 // indirect
	github.com/pmezard/go-difflib v1.0.0 // indirect
	github.com/remyoudompheng/bigfft v0.0.0-20230129092748-24d4a6f8daec // indirect
	github.com/spf13/pflag v1.0.7 // indirect
	github.com/tklauser/go-sysconf v0.3.15 // indirect
	github.com/tklauser/numcpus v0.10.0 // indirect
	github.com/yusufpapurcu/wmi v1.2.4 // indirect
	go.uber.org/automaxprocs v1.6.0 // indirect
	go.yaml.in/yaml/v3 v3.0.4 // indirect
	golang.org/x/mod v0.30.0 // indirect
	golang.org/x/net v0.47.0 // indirect
	golang.org/x/sync v0.18.0 // indirect
	golang.org/x/sys v0.38.0 // indirect
	golang.org/x/text v0.31.0 // indirect
	gopkg.in/yaml.v3 v3.0.1 // indirect
	lukechampine.com/uint128 v1.3.0 // indirect
	modernc.org/cc/v3 v3.41.0 // indirect
	modernc.org/ccgo/v3 v3.16.15 // indirect
	modernc.org/libc v1.37.6 // indirect
	modernc.org/mathutil v1.6.0 // indirect
	modernc.org/memory v1.7.2 // indirect
	modernc.org/opt v0.1.3 // indirect
	modernc.org/strutil v1.2.0 // indirect
	modernc.org/token v1.1.0 // indirect
)

// replace github.com/syifan/goseth => ../goseth

go 1.26.0

toolchain go1.26.2

// Retained dependency-security guard: removing this lets golang.org/x/net@v0.47.0
// select golang.org/x/crypto@v0.44.0 even though this module does not import it.
// Re-verify by deleting the exclude and running `go list -m golang.org/x/crypto`;
// remove the guard once the graph selects a later version on its own.
exclude golang.org/x/crypto v0.44.0

// Retained dependency-security guard: removing this makes go mod tidy -diff
// request the stale gopkg.in/yaml.v2 v2.2.2 go.mod checksum through an older
// testify path (tebeka/atexit -> testify v1.5.1). Re-verify by deleting the
// exclude and running `go mod tidy -diff`; remove the guard once no dependency
// path requests yaml.v2 v2.2.2.
exclude gopkg.in/yaml.v2 v2.2.2
