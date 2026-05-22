package gmmu

import (
	"testing"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/sarchlab/akita/v5/modeling"
)

//go:generate mockgen -destination "mock_engine_test.go" -package $GOPACKAGE -write_package_comment=false github.com/sarchlab/akita/v5/timing Engine
//go:generate mockgen -destination "mock_port_test.go" -package $GOPACKAGE -write_package_comment=false github.com/sarchlab/akita/v5/messaging Port
//go:generate mockgen -destination "mock_vm_test.go" -package $GOPACKAGE -write_package_comment=false github.com/sarchlab/akita/v5/mem/vm PageTable
//go:generate mockgen -destination "mock_mem_test.go" -package $GOPACKAGE -write_package_comment=false github.com/sarchlab/akita/v5/mem AddressToPortMapper
func TestGMMU(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "GMMU Suite")
}

func TestValidateState(t *testing.T) {
	if err := modeling.ValidateState(State{}); err != nil {
		t.Fatalf("ValidateState(State{}) = %v, want nil", err)
	}
}
