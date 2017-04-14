package core_test

import (
	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"

	"gitlab.com/yaotsu/core"
)

var _ = Describe("DirectConnection", func() {

	var (
		comp1      *core.MockComponent
		comp2      *core.MockComponent
		comp3      *core.MockComponent
		connection *core.DirectConnection
	)

	BeforeEach(func() {
		comp1 = core.NewMockComponent("comp1")
		comp2 = core.NewMockComponent("comp2")
		comp3 = core.NewMockComponent("comp3")

		connection = core.NewDirectConnection()
		connection.Attach(comp1)
		connection.Attach(comp2)
	})

	It("should give error is detaching a not attached component", func() {
		Expect(func() { connection.Detach(comp3) }).To(Panic())
	})

	It("should detach", func() {
		// Normal detaching
		Expect(func() { connection.Detach(comp1) }).NotTo(Panic())

		// Detaching again should give error
		Expect(func() { connection.Detach(comp1) }).To(Panic())
	})

	It("should give error if source is nil", func() {
		req := NewMockRequest()
		req.SetDst(comp2)

		err := connection.Send(req)
		Expect(err).NotTo(BeNil())
		Expect(err.Recoverable).To(BeFalse())
	})

	It("should give error if source is not connected", func() {
		req := NewMockRequest()
		req.SetDst(comp2)
		req.SetSrc(comp3)

		err := connection.Send(req)
		Expect(err).NotTo(BeNil())
		Expect(err.Recoverable).To(BeFalse())
	})

	It("should give error if destination is nil", func() {
		req := NewMockRequest()
		req.SetSrc(comp2)

		err := connection.Send(req)
		Expect(err).NotTo(BeNil())
		Expect(err.Recoverable).To(BeFalse())
	})

	It("should give error if destination is not connected", func() {
		req := NewMockRequest()
		req.SetSrc(comp2)
		req.SetDst(comp3)

		err := connection.Send(req)
		Expect(err).NotTo(BeNil())
		Expect(err.Recoverable).To(BeFalse())
	})

	It("should send", func() {
		req := NewMockRequest()
		req.SetSrc(comp2)
		req.SetDst(comp1)
		req.SetSendTime(2.0)

		errToRet := core.NewError("something", true, 10)
		comp1.ToReceiveReq(req, errToRet)

		err := connection.Send(req)
		Expect(err).To(BeIdenticalTo(errToRet))
		Expect(req.RecvTime()).To(Equal(core.VTimeInSec(2.0)))
	})

})
