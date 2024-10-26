module github.com/sarchlab/akita/v4

require (
	github.com/fatih/structs v1.1.0
	github.com/go-sql-driver/mysql v1.8.1
	github.com/golang/mock v1.6.0
	github.com/google/pprof v0.0.0-20240829160300-da1f7e9f2b25
	github.com/gorilla/mux v1.8.1
	github.com/mattn/go-sqlite3 v1.14.22
	github.com/onsi/ginkgo/v2 v2.20.2
	github.com/onsi/gomega v1.34.2
	github.com/rs/xid v1.6.0
	github.com/shirou/gopsutil v3.21.11+incompatible
	github.com/stretchr/testify v1.8.4
	github.com/syifan/goseth v0.1.2
	github.com/tebeka/atexit v0.3.0
)

require (
	filippo.io/edwards25519 v1.1.0 // indirect
	github.com/davecgh/go-spew v1.1.1 // indirect
	github.com/go-logr/logr v1.4.2 // indirect
	github.com/go-ole/go-ole v1.3.0 // indirect
	github.com/go-task/slim-sprig/v3 v3.0.0 // indirect
	github.com/google/go-cmp v0.6.0 // indirect
	github.com/kr/pretty v0.1.0 // indirect
	github.com/pmezard/go-difflib v1.0.0 // indirect
	github.com/tklauser/go-sysconf v0.3.14 // indirect
	github.com/tklauser/numcpus v0.8.0 // indirect
	github.com/yusufpapurcu/wmi v1.2.4 // indirect
	golang.org/x/net v0.28.0 // indirect
	golang.org/x/sys v0.24.0 // indirect
	golang.org/x/text v0.17.0 // indirect
	golang.org/x/tools v0.24.0 // indirect
	gopkg.in/check.v1 v1.0.0-20180628173108-788fd7840127 // indirect
	gopkg.in/yaml.v3 v3.0.1 // indirect
)

// replace github.com/syifan/goseth => ../goseth
go 1.22

toolchain go1.22.6
