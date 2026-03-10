package directconnection

//go:generate mockgen -destination "mock_sim_test.go" -self_package=github.com/sarchlab/akita/v5/sim/directconnection -package $GOPACKAGE -write_package_comment=false github.com/sarchlab/akita/v5/sim Port,Engine,Event,Connection,Component,Handler,Ticker

import (
	"testing"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

func TestDirectconnection(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "Directconnection Suite")
}
