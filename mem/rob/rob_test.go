package rob

import (
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/sarchlab/akita/v5/hooking"
	"github.com/sarchlab/akita/v5/mem"
	"github.com/sarchlab/akita/v5/mem/control"
	"github.com/sarchlab/akita/v5/messaging"
	"github.com/sarchlab/akita/v5/modeling"
	"github.com/sarchlab/akita/v5/timing"
)

// noopConn is a minimal messaging.Connection used to drive the reorder
// buffer's real ports in isolation. The ROB owns its ports, so tests feed
// requests via Deliver and consume sent messages via RetrieveOutgoing; the
// port still needs a connection so its send/retrieve notifications have
// somewhere to land.
type noopConn struct {
	hooking.HookableBase
}

func (c *noopConn) Name() string                     { return "NoopConn" }
func (c *noopConn) PlugIn(port messaging.Port)       { port.SetConnection(c) }
func (c *noopConn) Unplug(_ messaging.Port)          {}
func (c *noopConn) NotifyAvailable(_ messaging.Port) {}
func (c *noopConn) NotifySend()                      {}

var _ = Describe("Reorder Buffer", func() {
	const (
		topRemote        = messaging.RemotePort("Agent")
		bottomUnitRemote = messaging.RemotePort("BottomUnit")
	)

	var (
		engine     timing.Engine
		rob        *Comp
		topPort    messaging.Port
		bottomPort messaging.Port
		ctrlPort   messaging.Port

		topBufSize    int
		bottomBufSize int
		ctrlBufSize   int
	)

	build := func(spec Spec) {
		reg := modeling.NewStandaloneRegistrar(engine)

		rob = MakeBuilder().
			WithRegistrar(reg).
			WithSpec(spec).
			Build("Rob")

		assign := func(name string, bufSize int) messaging.Port {
			p := modeling.MakePortBuilder().
				WithRegistrar(reg).
				WithComponent(rob).
				WithSpec(modeling.PortSpec{BufSize: bufSize}).
				Build(name)
			rob.AssignPort(name, p)
			return p
		}

		topPort = assign("Top", topBufSize)
		bottomPort = assign("Bottom", bottomBufSize)
		ctrlPort = assign("Control", ctrlBufSize)

		for _, p := range []messaging.Port{topPort, bottomPort, ctrlPort} {
			conn := &noopConn{}
			conn.PlugIn(p)
		}
	}

	makeRead := func(addr uint64) mem.ReadReq {
		req := mem.ReadReq{
			Address:        addr,
			AccessByteSize: 4,
		}
		req.ID = timing.GetIDGenerator().Generate()
		req.Src = topRemote
		req.Dst = topPort.AsRemote()
		req.TrafficBytes = 12
		req.TrafficClass = "mem.ReadReq"
		return req
	}

	makeWrite := func(addr uint64, data []byte) mem.WriteReq {
		req := mem.WriteReq{
			Address: addr,
			Data:    data,
		}
		req.ID = timing.GetIDGenerator().Generate()
		req.Src = topRemote
		req.Dst = topPort.AsRemote()
		req.TrafficBytes = len(data) + 12
		req.TrafficClass = "mem.WriteReq"
		return req
	}

	BeforeEach(func() {
		engine = timing.NewSerialEngine()
		spec := DefaultSpec()
		spec.BufferSize = 4
		spec.NumReqPerCycle = 2
		spec.BottomUnit = bottomUnitRemote
		topBufSize = 4
		bottomBufSize = 4
		ctrlBufSize = 2
		build(spec)
	})

	Context("top-down", func() {
		It("forwards a read to the bottom and records the transaction", func() {
			req := makeRead(0)
			topPort.Deliver(req)

			progress := rob.Tick()

			Expect(progress).To(BeTrue())
			Expect(rob.State.Transactions).To(HaveLen(1))
			Expect(rob.State.Transactions[0].IsRead).To(BeTrue())
			Expect(rob.State.Transactions[0].ReqFromTopID).To(Equal(req.ID))
			Expect(rob.State.Transactions[0].ReqFromTopSrc).To(Equal(topRemote))

			sent := bottomPort.RetrieveOutgoing()
			Expect(sent).To(BeAssignableToTypeOf(mem.ReadReq{}))
			shadow := sent.(mem.ReadReq)
			Expect(shadow.Src).To(Equal(bottomPort.AsRemote()))
			Expect(shadow.Dst).To(Equal(bottomUnitRemote))
			Expect(shadow.Address).To(Equal(uint64(0)))
			Expect(shadow.AccessByteSize).To(Equal(uint64(4)))
			Expect(shadow.ID).To(Equal(rob.State.Transactions[0].ReqToBottomID))
			Expect(shadow.ID).ToNot(Equal(req.ID))
		})

		It("forwards a write to the bottom and records the transaction", func() {
			req := makeWrite(64, []byte{1, 2, 3, 4})
			topPort.Deliver(req)

			progress := rob.Tick()

			Expect(progress).To(BeTrue())
			Expect(rob.State.Transactions).To(HaveLen(1))
			Expect(rob.State.Transactions[0].IsRead).To(BeFalse())

			sent := bottomPort.RetrieveOutgoing()
			Expect(sent).To(BeAssignableToTypeOf(mem.WriteReq{}))
			shadow := sent.(mem.WriteReq)
			Expect(shadow.Data).To(Equal([]byte{1, 2, 3, 4}))
			Expect(shadow.Dst).To(Equal(bottomUnitRemote))
		})

		It("stalls when the buffer is full", func() {
			spec := rob.Spec()
			for i := 0; i < spec.BufferSize; i++ {
				rob.State.Transactions = append(rob.State.Transactions,
					transactionState{IsRead: true})
			}

			topPort.Deliver(makeRead(0))

			progress := rob.Tick()

			Expect(progress).To(BeFalse())
			Expect(rob.State.Transactions).To(HaveLen(spec.BufferSize))
			Expect(topPort.PeekIncoming()).ToNot(BeNil())
		})

		It("stalls when the bottom port is full", func() {
			topPort.Deliver(makeRead(0))

			// Fill the bottom outgoing buffer so Send fails.
			for i := 0; i < bottomBufSize; i++ {
				filler := mem.ReadReq{Address: uint64(i)}
				filler.ID = timing.GetIDGenerator().Generate()
				filler.Src = bottomPort.AsRemote()
				filler.Dst = bottomUnitRemote
				filler.TrafficClass = "mem.ReadReq"
				Expect(bottomPort.CanSend()).To(BeTrue())
				bottomPort.Send(filler)
			}

			progress := rob.Tick()

			Expect(progress).To(BeFalse())
			Expect(rob.State.Transactions).To(BeEmpty())
			Expect(topPort.PeekIncoming()).ToNot(BeNil())
		})

		It("panics on unsupported top-port traffic", func() {
			req := mem.ControlReq{Command: mem.CmdFlush}
			req.ID = timing.GetIDGenerator().Generate()
			req.Src = topRemote
			req.Dst = topPort.AsRemote()
			req.TrafficClass = "mem.ControlReq"
			topPort.Deliver(req)

			Expect(func() { rob.Tick() }).To(Panic())
		})
	})

	Context("parse bottom", func() {
		It("attaches the bottom response to the matching transaction", func() {
			req := makeWrite(0, []byte{0xAA})
			topPort.Deliver(req)
			rob.Tick() // forward to bottom

			Expect(rob.State.Transactions).To(HaveLen(1))
			shadowID := rob.State.Transactions[0].ReqToBottomID

			rsp := mem.WriteDoneRsp{}
			rsp.ID = timing.GetIDGenerator().Generate()
			rsp.Src = bottomUnitRemote
			rsp.Dst = bottomPort.AsRemote()
			rsp.RspTo = shadowID
			rsp.TrafficClass = "mem.WriteDoneRsp"
			bottomPort.Deliver(rsp)

			rob.Tick()

			// The response is recorded on this tick; bottomUp will drain
			// the head on the following tick (hardware pipeline ordering).
			Expect(rob.State.Transactions).To(HaveLen(1))
			Expect(rob.State.Transactions[0].HasRsp).To(BeTrue())
		})

		It("ignores a response that does not match any transaction", func() {
			rsp := mem.WriteDoneRsp{}
			rsp.ID = timing.GetIDGenerator().Generate()
			rsp.Src = bottomUnitRemote
			rsp.Dst = bottomPort.AsRemote()
			rsp.RspTo = 999999
			rsp.TrafficClass = "mem.WriteDoneRsp"
			bottomPort.Deliver(rsp)

			rob.Tick()

			Expect(bottomPort.PeekIncoming()).To(BeNil())
		})

		It("drops unsupported bottom-port traffic", func() {
			req := mem.ControlReq{Command: mem.CmdFlush}
			req.ID = timing.GetIDGenerator().Generate()
			req.Src = bottomUnitRemote
			req.Dst = bottomPort.AsRemote()
			req.TrafficClass = "mem.ControlReq"
			bottomPort.Deliver(req)

			progress := rob.Tick()

			Expect(progress).To(BeTrue())
			Expect(bottomPort.PeekIncoming()).To(BeNil())
			Expect(rob.State.Transactions).To(BeEmpty())
		})
	})

	Context("bottom up", func() {
		It("forwards the head response to the top once it is ready", func() {
			req := makeRead(0)
			topPort.Deliver(req)
			rob.Tick()
			shadowSent := bottomPort.RetrieveOutgoing()
			Expect(shadowSent).ToNot(BeNil())

			shadowID := rob.State.Transactions[0].ReqToBottomID

			rsp := mem.DataReadyRsp{Data: []byte{0xDE, 0xAD}}
			rsp.ID = timing.GetIDGenerator().Generate()
			rsp.Src = bottomUnitRemote
			rsp.Dst = bottomPort.AsRemote()
			rsp.RspTo = shadowID
			rsp.TrafficClass = "mem.DataReadyRsp"
			bottomPort.Deliver(rsp)

			rob.Tick() // parseBottom records the response
			rob.Tick() // bottomUp drains the head

			Expect(rob.State.Transactions).To(BeEmpty())

			topOut := topPort.RetrieveOutgoing()
			Expect(topOut).To(BeAssignableToTypeOf(mem.DataReadyRsp{}))
			data := topOut.(mem.DataReadyRsp)
			Expect(data.Data).To(Equal([]byte{0xDE, 0xAD}))
			Expect(data.RspTo).To(Equal(req.ID))
			Expect(data.Dst).To(Equal(topRemote))
			Expect(data.Src).To(Equal(topPort.AsRemote()))
		})

		It("preserves FIFO order across multiple in-flight transactions", func() {
			r1 := makeRead(0x100)
			r2 := makeRead(0x200)
			topPort.Deliver(r1)
			topPort.Deliver(r2)

			// Tick once: both requests forward to the bottom in the same
			// cycle (NumReqPerCycle=2).
			rob.Tick()
			Expect(rob.State.Transactions).To(HaveLen(2))
			Expect(bottomPort.RetrieveOutgoing()).ToNot(BeNil())
			Expect(bottomPort.RetrieveOutgoing()).ToNot(BeNil())

			shadow1 := rob.State.Transactions[0].ReqToBottomID
			shadow2 := rob.State.Transactions[1].ReqToBottomID

			// Deliver the second response first; the head must still wait
			// since its response has not arrived yet.
			rsp2 := mem.DataReadyRsp{Data: []byte{0x22}}
			rsp2.ID = timing.GetIDGenerator().Generate()
			rsp2.Src = bottomUnitRemote
			rsp2.Dst = bottomPort.AsRemote()
			rsp2.RspTo = shadow2
			rsp2.TrafficClass = "mem.DataReadyRsp"
			bottomPort.Deliver(rsp2)

			rob.Tick() // parseBottom records rsp2
			rob.Tick() // bottomUp finds head not ready, makes no progress

			Expect(rob.State.Transactions).To(HaveLen(2))
			Expect(rob.State.Transactions[0].HasRsp).To(BeFalse())
			Expect(rob.State.Transactions[1].HasRsp).To(BeTrue())
			Expect(topPort.RetrieveOutgoing()).To(BeNil())

			// Now deliver the response for the head; both should drain in
			// order on subsequent ticks.
			rsp1 := mem.DataReadyRsp{Data: []byte{0x11}}
			rsp1.ID = timing.GetIDGenerator().Generate()
			rsp1.Src = bottomUnitRemote
			rsp1.Dst = bottomPort.AsRemote()
			rsp1.RspTo = shadow1
			rsp1.TrafficClass = "mem.DataReadyRsp"
			bottomPort.Deliver(rsp1)

			rob.Tick() // parseBottom records rsp1
			rob.Tick() // bottomUp drains both (NumReqPerCycle=2)

			Expect(rob.State.Transactions).To(BeEmpty())

			out1 := topPort.RetrieveOutgoing()
			out2 := topPort.RetrieveOutgoing()
			Expect(out1).To(BeAssignableToTypeOf(mem.DataReadyRsp{}))
			Expect(out2).To(BeAssignableToTypeOf(mem.DataReadyRsp{}))
			Expect(out1.(mem.DataReadyRsp).Data).To(Equal([]byte{0x11}))
			Expect(out2.(mem.DataReadyRsp).Data).To(Equal([]byte{0x22}))
			Expect(out1.(mem.DataReadyRsp).RspTo).To(Equal(r1.ID))
			Expect(out2.(mem.DataReadyRsp).RspTo).To(Equal(r2.ID))
		})

		It("stalls when the top port cannot accept the response", func() {
			req := makeRead(0)
			topPort.Deliver(req)
			rob.Tick()
			bottomPort.RetrieveOutgoing()

			shadowID := rob.State.Transactions[0].ReqToBottomID
			rsp := mem.DataReadyRsp{Data: []byte{0x1}}
			rsp.ID = timing.GetIDGenerator().Generate()
			rsp.Src = bottomUnitRemote
			rsp.Dst = bottomPort.AsRemote()
			rsp.RspTo = shadowID
			rsp.TrafficClass = "mem.DataReadyRsp"
			bottomPort.Deliver(rsp)

			rob.Tick() // parseBottom records the response

			// Fill the top outgoing buffer so the next bottomUp Send fails.
			for i := 0; i < topBufSize; i++ {
				filler := mem.DataReadyRsp{Data: []byte{byte(i)}}
				filler.ID = timing.GetIDGenerator().Generate()
				filler.Src = topPort.AsRemote()
				filler.Dst = topRemote
				filler.TrafficClass = "mem.DataReadyRsp"
				Expect(topPort.CanSend()).To(BeTrue())
				topPort.Send(filler)
			}

			rob.Tick()

			// The head should still be present and marked HasRsp because
			// bottomUp could not enqueue the outgoing response.
			Expect(rob.State.Transactions).To(HaveLen(1))
			Expect(rob.State.Transactions[0].HasRsp).To(BeTrue())
		})
	})

	Context("control", func() {
		It("drops in-flight transactions and pauses on CmdReset", func() {
			topPort.Deliver(makeRead(0))
			rob.Tick()
			bottomPort.RetrieveOutgoing()
			Expect(rob.State.Transactions).To(HaveLen(1))

			req := mem.ControlReq{
				Command: mem.CmdReset,
			}
			req.ID = timing.GetIDGenerator().Generate()
			req.Src = messaging.RemotePort("Cmd")
			req.Dst = ctrlPort.AsRemote()
			req.TrafficClass = "mem.ControlReq"
			ctrlPort.Deliver(req)

			rob.Tick()

			Expect(rob.State.Transactions).To(BeEmpty())
			Expect(rob.State.ControlState).To(Equal(control.StateEnabled))

			ack := ctrlPort.RetrieveOutgoing()
			Expect(ack).To(BeAssignableToTypeOf(mem.ControlRsp{}))
			rsp := ack.(mem.ControlRsp)
			Expect(rsp.Command).To(Equal(mem.CmdReset))
			Expect(rsp.Success).To(BeTrue())
			Expect(rsp.RspTo).To(Equal(req.ID))
		})

		It("freezes the data pipeline while paused", func() {
			rob.State.ControlState = control.StatePaused
			topPort.Deliver(makeRead(0))

			progress := rob.Tick()

			Expect(progress).To(BeFalse())
			Expect(rob.State.Transactions).To(BeEmpty())
			Expect(topPort.PeekIncoming()).ToNot(BeNil())
		})

		It("resumes on CmdEnable, draining incoming traffic", func() {
			rob.State.ControlState = control.StatePaused

			// Stale traffic that should be cleared on resume.
			topPort.Deliver(makeRead(0))
			stray := mem.DataReadyRsp{Data: []byte{0xFF}}
			stray.ID = timing.GetIDGenerator().Generate()
			stray.Src = bottomUnitRemote
			stray.Dst = bottomPort.AsRemote()
			stray.RspTo = 0xDEAD
			stray.TrafficClass = "mem.DataReadyRsp"
			bottomPort.Deliver(stray)

			req := mem.ControlReq{Command: mem.CmdEnable}
			req.ID = timing.GetIDGenerator().Generate()
			req.Src = messaging.RemotePort("Cmd")
			req.Dst = ctrlPort.AsRemote()
			req.TrafficClass = "mem.ControlReq"
			ctrlPort.Deliver(req)

			rob.Tick()

			Expect(rob.State.ControlState).To(Equal(control.StateEnabled))
			Expect(topPort.PeekIncoming()).To(BeNil())
			Expect(bottomPort.PeekIncoming()).To(BeNil())

			ack := ctrlPort.RetrieveOutgoing()
			Expect(ack).To(BeAssignableToTypeOf(mem.ControlRsp{}))
			Expect(ack.(mem.ControlRsp).Command).To(Equal(mem.CmdEnable))
		})

		makeCtrlReq := func(cmd mem.ControlCommand) mem.ControlReq {
			req := mem.ControlReq{Command: cmd}
			req.ID = timing.GetIDGenerator().Generate()
			req.Src = messaging.RemotePort("Cmd")
			req.Dst = ctrlPort.AsRemote()
			req.TrafficClass = "mem.ControlReq"
			return req
		}

		It("acks Drain only after in-flight transactions retire", func() {
			const n = 2
			for i := range n {
				topPort.Deliver(makeRead(uint64(i * 0x100)))
			}
			rob.Tick() // forward both to the bottom (NumReqPerCycle=2)
			Expect(rob.State.Transactions).To(HaveLen(n))

			shadowIDs := make([]uint64, 0, n)
			for i := range rob.State.Transactions {
				shadowIDs = append(shadowIDs,
					rob.State.Transactions[i].ReqToBottomID)
			}
			for bottomPort.RetrieveOutgoing() != nil {
			}

			drain := makeCtrlReq(mem.CmdDrain)
			ctrlPort.Deliver(drain)

			// While transactions are still in flight (no bottom responses
			// fed yet), Drain must stay pending and emit no ack.
			for range 5 {
				rob.Tick()
				Expect(rob.State.ControlState).To(Equal(control.StateDraining))
				Expect(ctrlPort.RetrieveOutgoing()).To(BeNil())
			}
			Expect(rob.State.Transactions).To(HaveLen(n))

			// Now let the in-flight reads complete.
			for _, id := range shadowIDs {
				rsp := mem.DataReadyRsp{Data: []byte{0x1}}
				rsp.ID = timing.GetIDGenerator().Generate()
				rsp.Src = bottomUnitRemote
				rsp.Dst = bottomPort.AsRemote()
				rsp.RspTo = id
				rsp.TrafficClass = "mem.DataReadyRsp"
				bottomPort.Deliver(rsp)
			}

			completed := 0
			var drainRsp mem.ControlRsp
			drainFound := false
			for i := 0; i < 64 && !drainFound; i++ {
				rob.Tick()
				for {
					out := topPort.RetrieveOutgoing()
					if out == nil {
						break
					}
					if _, ok := out.(mem.DataReadyRsp); ok {
						completed++
					}
				}
				if out := ctrlPort.RetrieveOutgoing(); out != nil {
					if rsp, ok := out.(mem.ControlRsp); ok &&
						rsp.Command == mem.CmdDrain {
						drainRsp = rsp
						drainFound = true
					}
				}
			}

			Expect(drainFound).To(BeTrue())
			Expect(drainRsp.Success).To(BeTrue())
			Expect(drainRsp.RspTo).To(Equal(drain.ID))
			// All in-flight reads finished before the async Drain ack.
			Expect(completed).To(Equal(n))
			Expect(rob.State.Transactions).To(BeEmpty())
			Expect(rob.State.ControlState).To(Equal(control.StatePaused))
		})

		DescribeTable("Reset wipes in-flight transactions from any state",
			func(startState control.State) {
				topPort.Deliver(makeRead(0))
				rob.Tick()
				Expect(rob.State.Transactions).To(HaveLen(1))

				rob.State.ControlState = startState

				reset := makeCtrlReq(mem.CmdReset)
				ctrlPort.Deliver(reset)

				var rsp mem.ControlRsp
				found := false
				for i := 0; i < 64 && !found; i++ {
					rob.Tick()
					if out := ctrlPort.RetrieveOutgoing(); out != nil {
						rsp, found = out.(mem.ControlRsp)
					}
				}

				Expect(found).To(BeTrue())
				Expect(rsp.Command).To(Equal(mem.CmdReset))
				Expect(rsp.Success).To(BeTrue())
				Expect(rsp.RspTo).To(Equal(reset.ID))
				Expect(rob.State.Transactions).To(BeEmpty())
				Expect(rob.State.ControlState).To(Equal(control.StateEnabled))
			},
			Entry("from Enabled", control.StateEnabled),
			Entry("from Paused", control.StatePaused),
			// The draining case is covered separately by the test below, since
			// control commands are serialized: a Reset queued behind an
			// in-progress Drain is only serviced after the Drain acks.
		)

		It("completes a pending Drain before servicing a queued Reset", func() {
			// Draining and already quiescent (no in-flight transactions):
			// completePendingDrain acks the Drain. Control commands are
			// serialized with no preemption, so a Reset queued behind the
			// drain is serviced only after the Drain acks.
			rob.State.ControlState = control.StateDraining
			rob.State.CurrentCmdID = 999
			rob.State.CurrentCmdSrc = messaging.RemotePort("Drainer")
			rob.State.Transactions = nil

			reset := makeCtrlReq(mem.CmdReset)
			ctrlPort.Deliver(reset)

			var rsps []mem.ControlRsp
			for range 16 {
				rob.Tick()
				for {
					out := ctrlPort.RetrieveOutgoing()
					if out == nil {
						break
					}
					if r, ok := out.(mem.ControlRsp); ok {
						rsps = append(rsps, r)
					}
				}
			}

			Expect(rsps).To(HaveLen(2))
			Expect(rsps[0].Command).To(Equal(mem.CmdDrain))
			Expect(rsps[0].RspTo).To(Equal(uint64(999)))
			Expect(rsps[1].Command).To(Equal(mem.CmdReset))
			Expect(rsps[1].RspTo).To(Equal(reset.ID))
			Expect(rob.State.ControlState).To(Equal(control.StateEnabled))
		})
	})
})
