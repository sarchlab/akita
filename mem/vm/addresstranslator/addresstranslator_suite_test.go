package addresstranslator

import (
	"testing"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

//go:generate mockgen -destination "mock_sim_test.go" -package $GOPACKAGE -write_package_comment=false github.com/sarchlab/akita/v4/sim Port,Engine,BufferedSender
//go:generate mockgen -destination "mock_mem_test.go" -package $GOPACKAGE -write_package_comment=false github.com/sarchlab/akita/v4/mem/mem LowModuleFinder
func TestAddresstranslator(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "Address Translator Suite")
}
