module github.com/sarchlab/akita/v5

require (
	github.com/glebarez/go-sqlite v1.23.0
	github.com/google/pprof v0.0.0-20260825171938-4d453200e7d9
	github.com/mattn/go-sqlite3 v1.14.50
	github.com/onsi/ginkgo/v2 v2.32.1
	github.com/onsi/gomega v1.43.0
	github.com/pkg/browser v0.0.0-20240102092130-5ac0b6a4141c
	github.com/rs/xid v1.6.0
	github.com/shirou/gopsutil/v4 v4.26.7
	github.com/spf13/cobra v1.10.2
	github.com/stretchr/testify v1.12.1
	github.com/syifan/goseth v0.1.2
	github.com/tebeka/atexit v0.3.0
	go.uber.org/mock v0.6.0
	golang.org/x/tools v0.49.0
	modernc.org/sqlite v1.57.0
)

require (
	github.com/Masterminds/semver/v3 v3.5.0 // indirect
	github.com/dustin/go-humanize v1.0.1 // indirect
	github.com/ebitengine/purego v0.10.2 // indirect
	github.com/go-logr/logr v1.4.4 // indirect
	github.com/go-ole/go-ole v1.3.0 // indirect
	github.com/go-task/slim-sprig/v3 v3.0.0 // indirect
	github.com/google/go-cmp v0.7.0 // indirect
	github.com/google/uuid v1.6.0 // indirect
	github.com/inconshreveable/mousetrap v1.1.0 // indirect
	github.com/lufia/plan9stats v0.0.0-20211012122336-39d0f177ccd0 // indirect
	github.com/mattn/go-isatty v0.0.24 // indirect
	github.com/ncruces/go-strftime v1.0.0 // indirect
	github.com/power-devops/perfstat v0.0.0-20240221224432-82ca36839d55 // indirect
	github.com/remyoudompheng/bigfft v0.0.0-20230129092748-24d4a6f8daec // indirect
	github.com/spf13/pflag v1.0.10 // indirect
	github.com/tklauser/go-sysconf v0.4.0 // indirect
	github.com/tklauser/numcpus v0.12.0 // indirect
	github.com/yusufpapurcu/wmi v1.2.4 // indirect
	go.yaml.in/yaml/v3 v3.0.5 // indirect
	golang.org/x/mod v0.40.0 // indirect
	golang.org/x/net v0.58.0 // indirect
	golang.org/x/sync v0.22.0 // indirect
	golang.org/x/sys v0.47.0 // indirect
	golang.org/x/text v0.41.0 // indirect
	modernc.org/libc v1.75.6 // indirect
	modernc.org/mathutil v1.7.1 // indirect
	modernc.org/memory v1.12.1 // indirect
)

// replace github.com/syifan/goseth => ../goseth

go 1.27.0

// Retained dependency-security guard: tebeka/atexit still reaches testify
// v1.5.1's stale yaml.v2 requirement through its dependency graph. Remove this
// guard once `go list -m all` no longer selects yaml.v2 v2.2.2 without it.
exclude gopkg.in/yaml.v2 v2.2.2
