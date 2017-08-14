package connections

import (
	"log"
	"testing"

	"gitlab.com/yaotsu/core"

	"github.com/onsi/ginkgo"
	"github.com/onsi/gomega"
)

func TestCore(t *testing.T) {
	log.SetOutput(ginkgo.GinkgoWriter)
	gomega.RegisterFailHandler(ginkgo.Fail)
	ginkgo.RunSpecs(t, "Connections")
}

type MockRequest struct {
	*core.ReqBase
}

func NewMockRequest() *MockRequest {
	return &MockRequest{core.NewReqBase()}
}
