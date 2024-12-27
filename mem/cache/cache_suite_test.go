package cache

import (
	"testing"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

//go:generate mockgen -destination "mock_simulation_test.go" -package $GOPACKAGE -write_package_comment=false github.com/sarchlab/akita/v4/sim/simulation Simulation
//go:generate mockgen -destination "mock_mshr_test.go" -package $GOPACKAGE -write_package_comment=false github.com/sarchlab/akita/v4/mem/cache/internal/mshr MSHR
//go:generate mockgen -destination "mock_tags_test.go" -package $GOPACKAGE -write_package_comment=false github.com/sarchlab/akita/v4/mem/cache/internal/tagging VictimFinder
//go:generate mockgen -destination "mock_modeling_test.go" -package $GOPACKAGE -write_package_comment=false github.com/sarchlab/akita/v4/sim/modeling Port

func TestCache(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "Cache Suite")
}
