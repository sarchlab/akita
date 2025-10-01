package tracers

import (
	"testing"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

//go:generate mockgen -destination "mock_sim_test.go" -package $GOPACKAGE -write_package_comment=false github.com/sarchlab/akita/v4/sim TimeTeller
//go:generate mockgen -destination "mock_tracers_test.go" -package $GOPACKAGE -write_package_comment=false github.com/sarchlab/akita/v4/instrumentation/tracing/tracers TaskPrinter

func TestTracers(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "Tracers Suite")
}
