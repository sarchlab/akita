module github.com/sarchlab/akita/v4

require (
	github.com/go-sql-driver/mysql v1.8.1
	github.com/golang/mock v1.6.0
	github.com/google/pprof v0.0.0-20240424215950-a892ee059fd6
	github.com/gorilla/mux v1.8.1
	github.com/mattn/go-sqlite3 v1.14.22
	github.com/onsi/ginkgo/v2 v2.17.2
	github.com/onsi/gomega v1.33.1
	github.com/rs/xid v1.5.0
	github.com/shirou/gopsutil v3.21.11+incompatible
	github.com/syifan/goseth v0.1.2
	github.com/tebeka/atexit v0.3.0
)

require (
	filippo.io/edwards25519 v1.1.0 // indirect
	github.com/fsnotify/fsnotify v1.4.9 // indirect
	github.com/go-logr/logr v1.4.1 // indirect
	github.com/go-ole/go-ole v1.3.0 // indirect
	github.com/go-task/slim-sprig v0.0.0-20230315185526-52ccab3ef572 // indirect
	github.com/google/go-cmp v0.6.0 // indirect
	github.com/kr/pretty v0.1.0 // indirect
	github.com/nxadm/tail v1.4.8 // indirect
	github.com/onsi/ginkgo v1.16.5 // indirect
	github.com/tklauser/go-sysconf v0.3.13 // indirect
	github.com/tklauser/numcpus v0.7.0 // indirect
	github.com/yusufpapurcu/wmi v1.2.4 // indirect
	golang.org/x/net v0.24.0 // indirect
	golang.org/x/sys v0.19.0 // indirect
	golang.org/x/text v0.14.0 // indirect
	golang.org/x/tools v0.20.0 // indirect
	gopkg.in/check.v1 v1.0.0-20180628173108-788fd7840127 // indirect
	gopkg.in/tomb.v1 v1.0.0-20141024135613-dd632973f1e7 // indirect
	gopkg.in/yaml.v3 v3.0.1 // indirect
)

// replace github.com/syifan/goseth => ../goseth

go 1.20
