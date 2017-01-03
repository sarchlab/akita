package eventsys_test

import "testing"
import "github.com/onsi/gomega"
import "github.com/onsi/ginkgo"

func TestEventSys(t *testing.T) {
	gomega.RegisterFailHandler(ginkgo.Fail)
	ginkgo.RunSpecs(t, "Event System")
}
