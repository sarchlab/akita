package writethrough

import (
	"log"
	"testing"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/sarchlab/akita/v5/modeling"
)

//go:generate mockgen -destination "mock_cache_test.go" -package $GOPACKAGE -write_package_comment=false github.com/sarchlab/akita/v5/mem/cache Directory,MSHR
//go:generate mockgen -destination "mock_mem_test.go" -package $GOPACKAGE -write_package_comment=false github.com/sarchlab/akita/v5/mem/mem AddressToPortMapper
//go:generate mockgen -destination "mock_sim_test.go" -package $GOPACKAGE -write_package_comment=false github.com/sarchlab/akita/v5/sim Port
//go:generate mockgen -destination "mock_queueing_test.go" -package $GOPACKAGE -write_package_comment=false github.com/sarchlab/akita/v5/queueing Buffer,Pipeline
func TestL1v(t *testing.T) {
	log.SetOutput(GinkgoWriter)
	RegisterFailHandler(Fail)
	RunSpecs(t, "Write-Through Suite")
}

func TestValidateState(t *testing.T) {
	if err := modeling.ValidateState(State{}); err != nil {
		t.Fatalf("State failed validation: %v", err)
	}
}
