module github.com/sarchlab/akita/v4

require (
	github.com/google/pprof v0.0.0-20250820193118-f64d9cf942d6
	github.com/gorilla/mux v1.8.1
	github.com/joho/godotenv v1.5.1
	github.com/mattn/go-sqlite3 v1.14.32
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
)

require (
	github.com/Masterminds/semver/v3 v3.4.0 // indirect
	github.com/davecgh/go-spew v1.1.1 // indirect
	github.com/go-logr/logr v1.4.3 // indirect
	github.com/go-ole/go-ole v1.3.0 // indirect
	github.com/go-task/slim-sprig/v3 v3.0.0 // indirect
	github.com/google/go-cmp v0.7.0 // indirect
	github.com/inconshreveable/mousetrap v1.1.0 // indirect
	github.com/pmezard/go-difflib v1.0.0 // indirect
	github.com/spf13/pflag v1.0.7 // indirect
	github.com/tklauser/go-sysconf v0.3.15 // indirect
	github.com/tklauser/numcpus v0.10.0 // indirect
	github.com/yusufpapurcu/wmi v1.2.4 // indirect
	go.uber.org/automaxprocs v1.6.0 // indirect
	go.yaml.in/yaml/v3 v3.0.4 // indirect
	golang.org/x/net v0.43.0 // indirect
	golang.org/x/sys v0.35.0 // indirect
	golang.org/x/text v0.28.0 // indirect
	golang.org/x/tools v0.36.0 // indirect
	gopkg.in/yaml.v3 v3.0.1 // indirect
)

// replace github.com/syifan/goseth => ../goseth

go 1.24.0

toolchain go1.24.7
