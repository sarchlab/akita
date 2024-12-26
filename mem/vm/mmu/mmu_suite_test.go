package mmu

import (
	"testing"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

//go:generate mockgen -destination "mock_modeling_test.go" -package $GOPACKAGE -write_package_comment=false github.com/sarchlab/akita/v4/sim/modeling Port
//go:generate mockgen -destination "mock_timing_test.go" -package $GOPACKAGE -write_package_comment=false github.com/sarchlab/akita/v4/sim/timing Engine
//go:generate mockgen -destination "mock_mem_test.go" -package $GOPACKAGE -write_package_comment=false "github.com/sarchlab/akita/v4/mem" AddressToPortMapper
//go:generate mockgen -destination "mock_vm_test.go" -package $GOPACKAGE -write_package_comment=false github.com/sarchlab/akita/v4/mem/vm PageTable
func TestMMU(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "MMU Suite")
}
