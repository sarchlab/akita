package addresstranslator

import (
	"testing"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

//go:generate mockgen -destination "mock_sim_test.go" -package $GOPACKAGE -write_package_comment=false github.com/sarchlab/akita/v5/simulation/messaging Port
//go:generate mockgen -destination "mock_timing_test.go" -package $GOPACKAGE -write_package_comment=false github.com/sarchlab/akita/v5/simulation/timing Engine
func TestAddresstranslator(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "Address Translator Suite")
}
