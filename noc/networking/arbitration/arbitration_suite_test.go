package arbitration

import (
	"testing"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

//go:generate mockgen -destination "mock_queueing_test.go" -package $GOPACKAGE -write_package_comment=false github.com/sarchlab/akita/v4/sim/queueing Buffer
func TestArbitration(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "Arbitration Suite")
}
