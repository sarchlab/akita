package directconnection

//go:generate mockgen -destination "mock_sim_test.go" -self_package=github.com/sarchlab/akita/v5/noc/directconnection -package $GOPACKAGE -write_package_comment=false github.com/sarchlab/akita/v5/messaging Port,Connection,Component
//go:generate mockgen -destination "mock_timing_test.go" -self_package=github.com/sarchlab/akita/v5/noc/directconnection -package $GOPACKAGE -write_package_comment=false github.com/sarchlab/akita/v5/timing Engine,Event,Handler
//go:generate mockgen -destination "mock_modeling_test.go" -self_package=github.com/sarchlab/akita/v5/noc/directconnection -package $GOPACKAGE -write_package_comment=false github.com/sarchlab/akita/v5/modeling Ticker

import (
	"testing"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

func TestDirectconnection(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "Directconnection Suite")
}
