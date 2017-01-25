package requestsys_test

import (
	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
	"gitlab.com/syifan/yaotsu/requestsys"
)

var _ = Describe("ReqSink", func() {
	reqSink := new(requestsys.ReqSink)

	It("should always allow sending", func() {
		Expect(reqSink.CanSend(nil)).To(BeTrue())
	})

	It("should do nothing while sending", func() {
		reqSink.Send(nil)
	})

	It("should do nothing linking and unlinking socket", func() {
	})

})
