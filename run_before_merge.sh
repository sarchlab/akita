#!/bin/bash

set -o xtrace

go get -u ./...
go get -t -u ./...
go mod tidy

go install go.uber.org/mock/mockgen@v0.6.0
go generate ./...

go build ./...

curl -sfL https://raw.githubusercontent.com/golangci/golangci-lint/master/install.sh | sh -s -- -b "$(go env GOPATH)/bin" v2.9.0
golangci-lint run ./... --timeout=10m

go install github.com/onsi/ginkgo/v2/ginkgo@v2.25.1
ginkgo -r

