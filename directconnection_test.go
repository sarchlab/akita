package core

import (
	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
)

var _ = Describe("DirectConnection", func() {

	var (
		port1      *Port
		port2      *Port
		connection *DirectConnection
		engine     *MockEngine
	)

	BeforeEach(func() {
		port1 = NewPort(nil)
		port2 = NewPort(nil)
		engine = NewMockEngine()

		connection = NewDirectConnection(engine)
		connection.PlugIn(port1)
		connection.PlugIn(port2)
	})

	It("should buffer the req if receiver is busy", func() {
		connection.receiverBusy[port1] = true

		req := newMockRequest()
		req.SetSrc(port2)
		req.SetDst(port1)
		req.SetSendTime(2.0)

		err := connection.Send(req)

		Expect(err).To(BeNil())
		Expect(engine.ScheduledEvent).To(HaveLen(0))
		Expect(connection.reqBuf[port1]).To(HaveLen(1))
	})

	It("should buffer the req if there are more requests to send to the receiver", func() {

		req1 := newMockRequest()
		req1.SetSrc(port2)
		req1.SetDst(port1)
		req1.SetSendTime(2.0)
		connection.reqBuf[port1] = append(
			connection.reqBuf[port1], req1)

		req2 := newMockRequest()
		req2.SetSrc(port2)
		req2.SetDst(port1)
		req2.SetSendTime(2.0)

		err := connection.Send(req2)
		Expect(err).To(BeNil())
		Expect(engine.ScheduledEvent).To(HaveLen(0))
		Expect(connection.reqBuf[port1]).To(HaveLen(2))
	})

	It("should send", func() {
		req := newMockRequest()
		req.SetSrc(port2)
		req.SetDst(port1)
		req.SetSendTime(2.0)

		err := connection.Send(req)

		Expect(err).To(BeNil())
		Expect(engine.ScheduledEvent).To(HaveLen(1))
		Expect(connection.reqBuf[port1]).To(HaveLen(1))
	})

	It("should deliver req", func() {
		req := newMockRequest()
		req.SetSrc(port2)
		req.SetDst(port1)
		req.SetSendTime(2.0)
		connection.reqBuf[port1] = append(connection.reqBuf[port1], req)
		evt := NewDeliverEvent(3.0, connection, req)

		connection.Handle(evt)

		Expect(port1.Buf).To(HaveLen(1))
		Expect(req.RecvTime()).To(BeNumerically("~", 3.0, 1e-12))
		Expect(connection.reqBuf[port1]).To(HaveLen(0))
	})

	It("should scheduler more deliver event if there are buffered reqs", func() {
		req1 := newMockRequest()
		req1.SetSrc(port2)
		req1.SetDst(port1)
		req1.SetSendTime(2.0)
		connection.reqBuf[port1] = append(connection.reqBuf[port1], req1)

		req := newMockRequest()
		req.SetSrc(port2)
		req.SetDst(port1)
		req.SetSendTime(2.0)
		connection.reqBuf[port1] = append(connection.reqBuf[port1], req)
		evt := NewDeliverEvent(3.0, connection, req)

		connection.Handle(evt)

		Expect(port1.Buf).To(HaveLen(1))
		Expect(req.RecvTime()).To(BeNumerically("~", 3.0, 1e-12))
		Expect(engine.ScheduledEvent).To(HaveLen(1))
		Expect(connection.reqBuf[port1]).To(HaveLen(1))
	})

	It("should mark receiver busy if deliver is not successful", func() {
		req := newMockRequest()
		req.SetSrc(port2)
		req.SetDst(port1)
		req.SetSendTime(2.0)
		port1.Buf = make([]Req, port1.BufCapacity)
		connection.reqBuf[port1] = append(connection.reqBuf[port1], req)

		evt := NewDeliverEvent(2.0, connection, req)
		connection.Handle(evt)

		Expect(engine.ScheduledEvent).To(HaveLen(0))
		Expect(connection.reqBuf[port1]).To(HaveLen(1))
		Expect(connection.receiverBusy[port1]).To(BeTrue())
	})

	It("should retry delivery if the receiver becomes available", func() {
		req := newMockRequest()
		req.SetSrc(port2)
		req.SetDst(port1)
		req.SetSendTime(2.0)
		connection.receiverBusy[port1] = true
		connection.reqBuf[port1] = append(connection.reqBuf[port1], req)

		connection.NotifyAvailable(10.0, port1)

		Expect(engine.ScheduledEvent).To(HaveLen(1))
		Expect(engine.ScheduledEvent[0].Time()).To(BeNumerically("~", 10.0))
		Expect(connection.receiverBusy[port1]).To(BeFalse())
		Expect(connection.reqBuf[port1]).To(HaveLen(1))
	})

})
