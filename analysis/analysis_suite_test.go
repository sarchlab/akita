package analysis

import (
	"testing"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

//go:generate mockgen -destination "mock_sim_test.go" -package $GOPACKAGE -write_package_comment=false github.com/sarchlab/akita/v4/sim TimeTeller,Port,Buffer
//go:generate mockgen -destination "mock_analysis_test.go" -package $GOPACKAGE -write_package_comment=false -source=perf_analyzer.go PerfLogger

func TestAnalysis(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "Analysis Suite")
}
