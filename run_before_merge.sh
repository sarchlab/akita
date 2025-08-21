#!/bin/bash

set -o xtrace

go get -u ./...
go get -t -u ./...
go mod tidy

go generate ./...

go build ./...

curl -sfL https://raw.githubusercontent.com/golangci/golangci-lint/master/install.sh| sh -s -- -b $(go env GOPATH)/bin v2.1.5 
golangci-lint run ./...

go install github.com/onsi/ginkgo/v2/ginkgo@v2.25.0
ginkgo -r

