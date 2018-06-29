package core

import (
	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
)

var _ = Describe("DirectConnection", func() {

	var (
		comp1      *MockComponent
		comp2      *MockComponent
		connection *DirectConnection
		engine     *MockEngine
	)

	BeforeEach(func() {
		comp1 = NewMockComponent("comp1")
		comp2 = NewMockComponent("comp2")
		engine = NewMockEngine()

		connection = NewDirectConnection(engine)
		connection.PlugIn(comp1, "ToOutside")
		connection.PlugIn(comp2, "ToOutside")
	})

	It("should buffer the req if receiver is busy", func() {
		connection.receiverBusy[comp1] = true

		req := newMockRequest()
		req.SetSrc(comp2)
		req.SetDst(comp1)
		req.SetSendTime(2.0)

		err := connection.Send(req)

		Expect(err).To(BeNil())
		Expect(engine.ScheduledEvent).To(HaveLen(0))
		Expect(connection.reqBuf[comp1]).To(HaveLen(1))
	})

	It("should buffer the req if there are more requests to send to the receiver", func() {

		req1 := newMockRequest()
		req1.SetSrc(comp2)
		req1.SetDst(comp1)
		req1.SetSendTime(2.0)
		connection.reqBuf[comp1] = append(connection.reqBuf[comp1], req1)

		req2 := newMockRequest()
		req2.SetSrc(comp2)
		req2.SetDst(comp1)
		req2.SetSendTime(2.0)

		err := connection.Send(req2)
		Expect(err).To(BeNil())
		Expect(engine.ScheduledEvent).To(HaveLen(0))
		Expect(connection.reqBuf[comp1]).To(HaveLen(2))
	})

	It("should send", func() {
		req := newMockRequest()
		req.SetSrc(comp2)
		req.SetDst(comp1)
		req.SetSendTime(2.0)

		err := connection.Send(req)

		Expect(err).To(BeNil())
		Expect(engine.ScheduledEvent).To(HaveLen(1))
		Expect(connection.reqBuf[comp1]).To(HaveLen(1))
	})

	It("should deliver req", func() {
		req := newMockRequest()
		req.SetSrc(comp2)
		req.SetDst(comp1)
		req.SetSendTime(2.0)
		connection.reqBuf[comp1] = append(connection.reqBuf[comp1], req)
		evt := NewDeliverEvent(3.0, connection, req)

		comp1.ToReceiveReq(req, nil)
		connection.Handle(evt)

		Expect(comp1.AllReqReceived()).To(BeTrue())
		Expect(req.RecvTime()).To(BeNumerically("~", 3.0, 1e-12))
		Expect(connection.reqBuf[comp1]).To(HaveLen(0))
	})

	It("should scheduler more deliver event if there are buffered reqs", func() {
		req1 := newMockRequest()
		req1.SetSrc(comp2)
		req1.SetDst(comp1)
		req1.SetSendTime(2.0)
		connection.reqBuf[comp1] = append(connection.reqBuf[comp1], req1)

		req := newMockRequest()
		req.SetSrc(comp2)
		req.SetDst(comp1)
		req.SetSendTime(2.0)
		connection.reqBuf[comp1] = append(connection.reqBuf[comp1], req)
		evt := NewDeliverEvent(3.0, connection, req)

		comp1.ToReceiveReq(req, nil)
		connection.Handle(evt)

		Expect(comp1.AllReqReceived()).To(BeTrue())
		Expect(req.RecvTime()).To(BeNumerically("~", 3.0, 1e-12))
		Expect(engine.ScheduledEvent).To(HaveLen(1))
		Expect(connection.reqBuf[comp1]).To(HaveLen(1))
	})

	It("should mark receiver busy if deliver is not successful", func() {
		req := newMockRequest()
		req.SetSrc(comp2)
		req.SetDst(comp1)
		req.SetSendTime(2.0)
		evt := NewDeliverEvent(2.0, connection, req)
		connection.reqBuf[comp1] = append(connection.reqBuf[comp1], req)

		comp1.ToReceiveReq(req, NewSendError())
		connection.Handle(evt)

		Expect(engine.ScheduledEvent).To(HaveLen(0))
		Expect(connection.reqBuf[comp1]).To(HaveLen(1))
		Expect(connection.receiverBusy[comp1]).To(BeTrue())
	})

	It("should retry delivery if the receiver becomes available", func() {
		req := newMockRequest()
		req.SetSrc(comp2)
		req.SetDst(comp1)
		req.SetSendTime(2.0)
		connection.receiverBusy[comp1] = true
		connection.reqBuf[comp1] = append(connection.reqBuf[comp1], req)

		connection.NotifyAvailable(10.0, comp1)

		Expect(engine.ScheduledEvent).To(HaveLen(1))
		Expect(engine.ScheduledEvent[0].Time()).To(BeNumerically("~", 10.0))
		Expect(connection.receiverBusy[comp1]).To(BeFalse())
		Expect(connection.reqBuf[comp1]).To(HaveLen(1))
	})

})
