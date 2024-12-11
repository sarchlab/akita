package directconnection

//go:generate mockgen -destination "mock_timing_test.go" -self_package=github.com/sarchlab/akita/v4/sim/directconnection -package $GOPACKAGE -write_package_comment=false github.com/sarchlab/akita/v4/sim/timing Engine,Event,Handler,Ticker
//go:generate mockgen -destination "mock_model_test.go" -self_package=github.com/sarchlab/akita/v4/sim/directconnection -package $GOPACKAGE -write_package_comment=false github.com/sarchlab/akita/v4/sim/model Port,Connection,Component

import (
	"testing"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

func TestDirectconnection(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "Directconnection Suite")
}
