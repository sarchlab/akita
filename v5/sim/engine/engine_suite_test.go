package engine

import (
	"log"
	"testing"

	"github.com/onsi/ginkgo/v2"
	"github.com/onsi/gomega"
)

//go:generate mockgen -destination "mock_engine_test.go" -package engine -write_package_comment=false github.com/sarchlab/akita/v5/sim Event,Handler

func TestEngine(t *testing.T) {
	log.SetOutput(ginkgo.GinkgoWriter)
	gomega.RegisterFailHandler(ginkgo.Fail)
	ginkgo.RunSpecs(t, "Engine Suite")
}
