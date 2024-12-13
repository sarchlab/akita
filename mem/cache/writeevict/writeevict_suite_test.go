package writeevict

import (
	"log"
	"testing"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

//go:generate mockgen -destination "mock_cache_test.go" -package $GOPACKAGE -write_package_comment=false github.com/sarchlab/akita/v4/mem/cache Directory,MSHR
//go:generate mockgen -destination "mock_mem_test.go" -package $GOPACKAGE -write_package_comment=false github.com/sarchlab/akita/v4/mem/mem AddressToPortMapper
//go:generate mockgen -destination "mock_modeling_test.go" -package $GOPACKAGE -write_package_comment=false github.com/sarchlab/akita/v4/sim/modeling Port
//go:generate mockgen -destination "mock_queueing_test.go" -package $GOPACKAGE -write_package_comment=false "github.com/sarchlab/akita/v4/sim/queueing" Buffer,Pipeline
func TestWriteevict(t *testing.T) {
	log.SetOutput(GinkgoWriter)
	RegisterFailHandler(Fail)
	RunSpecs(t, "Writeevict Suite")
}
