package requestsys_test

import "testing"
import "github.com/onsi/gomega"
import "github.com/onsi/ginkgo"

func TestRequestSys(t *testing.T) {
	gomega.RegisterFailHandler(ginkgo.Fail)
	ginkgo.RunSpecs(t, "Request System")
}
