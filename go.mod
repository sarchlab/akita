module gitlab.com/akita/akita/v3

require (
	github.com/go-sql-driver/mysql v1.6.0
	github.com/golang/mock v1.4.4
	github.com/golang/snappy v0.0.2 // indirect
	github.com/google/go-cmp v0.5.6 // indirect
	github.com/gorilla/mux v1.8.0
	github.com/onsi/ginkgo/v2 v2.1.3
	github.com/onsi/gomega v1.17.0
	github.com/rs/xid v1.2.1
	github.com/syifan/goseth v0.0.4
	github.com/tebeka/atexit v0.3.0
	github.com/tidwall/pretty v1.0.2 // indirect
	go.mongodb.org/mongo-driver v1.8.4
	gopkg.in/yaml.v3 v3.0.0-20200615113413-eeeca48fe776 // indirect
)

// replace github.com/syifan/goseth => ../goseth

go 1.16
