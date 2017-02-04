package conn_test

import (
	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
	"gitlab.com/yaotsu/core/conn"
)

var _ = Describe("ReqSink", func() {
	reqSink := new(conn.ReqSink)

	It("should always allow sending", func() {
		Expect(reqSink.CanSend(nil)).To(BeNil())
	})

	It("should do nothing while sending", func() {
		Expect(reqSink.Send(nil)).To(BeNil())
	})

	It("should do nothing linking and unlinking socket", func() {
		reqSink.Register(nil)
		reqSink.Unregister(nil)
	})

})
