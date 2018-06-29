package core

//import (
//	. "github.com/onsi/ginkgo"
//	. "github.com/onsi/gomega"
//
//	"gitlab.com/yaotsu/core"
//	"gitlab.com/yaotsu/core/util"
//)
//
//var _ = Describe("FixedLatencyConnection", func() {
//
//	var (
//		comp1      *core.MockComponent
//		comp2      *core.MockComponent
//		comp3      *core.MockComponent
//		freq       util.Freq
//		connection *FixedLatencyConnection
//		engine     *core.MockEngine
//	)
//
//	BeforeEach(func() {
//		comp1 = core.NewMockComponent("comp1")
//		comp2 = core.NewMockComponent("comp2")
//		comp3 = core.NewMockComponent("comp3")
//		engine = core.NewMockEngine()
//
//		freq = 1 * util.GHz
//		latency := 2
//		connection = NewFixedLatencyConnection(engine, latency, freq)
//		connection.Attach(comp1)
//		connection.Attach(comp2)
//	})
//
//	It("should give error is detaching a not attached component", func() {
//		Expect(func() { connection.Detach(comp3) }).To(Panic())
//	})
//
//	It("should detach", func() {
//		// Normal detaching
//		Expect(func() { connection.Detach(comp1) }).NotTo(Panic())
//
//		// Detaching again should give error
//		Expect(func() { connection.Detach(comp1) }).To(Panic())
//	})
//
//	It("should send with latency", func() {
//		req := newMockRequest()
//		req.SetSrc(comp2)
//		req.SetDst(comp1)
//		req.SetSendTime(2.0)
//
//		connection.Send(req)
//
//		Expect(len(engine.ScheduledEvent)).To(Equal(1))
//		Expect(engine.ScheduledEvent[0].Time()).To(
//			BeNumerically("~", 2.000000002, 1e-12))
//	})
//
//	It("should deliever", func() {
//		req := newMockRequest()
//		req.SetDst(comp1)
//		req.SetSendTime(2.0)
//		evt := NewDeliverEvent(2.0, connection, req)
//
//		comp1.ToReceiveReq(req, nil)
//
//		connection.Handle(evt)
//
//		Expect(comp1.AllReqReceived()).To(BeTrue())
//	})
//
//	It("should reschedule delievery if error", func() {
//		req := newMockRequest()
//		req.SetDst(comp1)
//		req.SetSendTime(2.0)
//		evt := NewDeliverEvent(2.0, connection, req)
//
//		comp1.ToReceiveReq(req, core.NewError("", true, 2.2))
//
//		connection.Handle(evt)
//
//		Expect(comp1.AllReqReceived()).To(BeTrue())
//		Expect(len(engine.ScheduledEvent)).To(Equal(1))
//		Expect(engine.ScheduledEvent[0].Time()).To(Equal(core.VTimeInSec(2.2)))
//
//	})
//
//})
