package bottleneckanalysis

import (
	"testing"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

//go:generate mockgen -destination "mock_sim_test.go" -package $GOPACKAGE -write_package_comment=false gitlab.com/akita/akita/v3/sim TimeTeller

func TestBottleneckanalysis(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "Bottleneckanalysis Suite")
}
