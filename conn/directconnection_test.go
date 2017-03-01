package conn_test

import (
	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"

	"gitlab.com/yaotsu/core/conn"
	"gitlab.com/yaotsu/core/event"
)

var _ = Describe("DirectConnection", func() {

	var (
		comp1      *conn.MockComponent
		comp2      *conn.MockComponent
		comp3      *conn.MockComponent
		connection *conn.DirectConnection
	)

	BeforeEach(func() {
		comp1 = conn.NewMockComponent()
		comp2 = conn.NewMockComponent()
		comp3 = conn.NewMockComponent()

		connection = conn.NewDirectConnection()
		connection.Attach(comp1)
		connection.Attach(comp2)
	})

	It("should give error is detaching a not attached component", func() {
		err := connection.Detach(comp3)
		Expect(err).NotTo(BeNil())
	})

	It("should detach", func() {
		// Normal detaching
		err := connection.Detach(comp1)
		Expect(err).To(BeNil())

		// Detaching again should give error
		err = connection.Detach(comp1)
		Expect(err).NotTo(BeNil())
	})

	It("should give error if source is nil", func() {
		req := NewMockRequest()
		req.SetDestination(comp2)

		err := connection.Send(req)
		Expect(err).NotTo(BeNil())
		Expect(err.Recoverable).To(BeFalse())
	})

	It("should give error if source is not connected", func() {
		req := NewMockRequest()
		req.SetDestination(comp2)
		req.SetSource(comp3)

		err := connection.Send(req)
		Expect(err).NotTo(BeNil())
		Expect(err.Recoverable).To(BeFalse())
	})

	It("should give error if destination is nil", func() {
		req := NewMockRequest()
		req.SetSource(comp2)

		err := connection.Send(req)
		Expect(err).NotTo(BeNil())
		Expect(err.Recoverable).To(BeFalse())
	})

	It("should give error if destination is not connected", func() {
		req := NewMockRequest()
		req.SetSource(comp2)
		req.SetDestination(comp3)

		err := connection.Send(req)
		Expect(err).NotTo(BeNil())
		Expect(err.Recoverable).To(BeFalse())
	})

	It("should send", func() {
		req := NewMockRequest()
		req.SetSource(comp2)
		req.SetDestination(comp1)
		req.SetSendTime(2.0)

		errToRet := conn.NewError("something", true, 10)
		comp1.ToReceiveReq(req, errToRet)

		err := connection.Send(req)
		Expect(err).To(BeIdenticalTo(errToRet))
		Expect(req.RecvTime()).To(Equal(event.VTimeInSec(2.0)))
	})

})
