package tracing

import (
	"testing"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

//go:generate mockgen -destination "mock_sim_test.go" -package $GOPACKAGE -write_package_comment=false github.com/sarchlab/akita/v3/sim TimeTeller
//go:generate mockgen -destination "mock_tracing_test.go" -package $GOPACKAGE -write_package_comment=false github.com/sarchlab/akita/v3/tracing NamedHookable

func TestTracing(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "Tracing Suite")
}
