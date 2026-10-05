package rob

import (
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/sarchlab/akita/v5/mem/memcontrolprotocol"
	"github.com/sarchlab/akita/v5/mem/memprotocol"
	"github.com/sarchlab/akita/v5/simulation/hooking"
	"github.com/sarchlab/akita/v5/simulation/messaging"
	"github.com/sarchlab/akita/v5/simulation/modeling"
	"github.com/sarchlab/akita/v5/simulation/modeling/modelingtest"
	"github.com/sarchlab/akita/v5/simulation/timing"
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
		sim        timing.Simulation
		rob        *Comp
		topPort    messaging.Port
		bottomPort messaging.Port
		ctrlPort   messaging.Port

		topBufSize    int
		bottomBufSize int
		ctrlBufSize   int
	)

	build := func(spec Spec) {
		port := func(name string, bufSize int) messaging.Port {
			return messaging.NewPort("Rob."+name, bufSize, bufSize)
		}

		topPort = port("Top", topBufSize)
		bottomPort = port("Bottom", bottomBufSize)
		ctrlPort = port("Control", ctrlBufSize)

		rob = Definition.Builder().
			WithSimulation(sim).
			WithSpec(spec).
			WithPorts(Ports{Top: topPort, Bottom: bottomPort, Control: ctrlPort}).
			Build("Rob")

		for _, p := range []messaging.Port{topPort, bottomPort, ctrlPort} {
			conn := &noopConn{}
			conn.PlugIn(p)
		}
	}

	makeRead := func(addr uint64) messaging.Msg {
		req := messaging.Msg{Payload: memprotocol.ReadReq{
			Address:        addr,
			AccessByteSize: 4,
		},
			ID:           sim.NewID(),
			Src:          topRemote,
			Dst:          topPort.AsRemote(),
			TrafficBytes: 12,
			TrafficClass: "memprotocol.ReadReq"}

		return req
	}

	makeWrite := func(addr uint64, data []byte) messaging.Msg {
		req := messaging.Msg{Payload: memprotocol.WriteReq{
			Address: addr,
			Data:    data,
		},
			ID:           sim.NewID(),
			Src:          topRemote,
			Dst:          topPort.AsRemote(),
			TrafficBytes: len(data) + 12,
			TrafficClass: "memprotocol.WriteReq"}

		return req
	}

	BeforeEach(func() {
		engine = timing.NewSerialEngine()
		sim = modeling.NewStandaloneSimulation(engine)
		spec := Definition.DefaultSpec
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

			progress := modelingtest.Tick(rob)

			Expect(progress).To(BeTrue())
			Expect(rob.State.Transactions).To(HaveLen(1))
			Expect(rob.State.Transactions[0].IsRead).To(BeTrue())
			Expect(rob.State.Transactions[0].ReqFromTopID).To(Equal(req.ID))
			Expect(rob.State.Transactions[0].ReqFromTopSrc).To(Equal(topRemote))

			sent, _ := bottomPort.RetrieveOutgoing()
			Expect(sent).To(BeAssignableToTypeOf(messaging.Msg{Payload: memprotocol.ReadReq{}}))
			shadow := sent
			Expect(shadow.Src).To(Equal(bottomPort.AsRemote()))
			Expect(shadow.Dst).To(Equal(bottomUnitRemote))
			Expect(shadow.Payload.(memprotocol.ReadReq).Address).To(Equal(uint64(0)))
			Expect(shadow.Payload.(memprotocol.ReadReq).AccessByteSize).To(Equal(uint64(4)))
			Expect(shadow.ID).To(Equal(rob.State.Transactions[0].ReqToBottomID))
			Expect(shadow.ID).ToNot(Equal(req.ID))
		})

		It("forwards a write to the bottom and records the transaction", func() {
			req := makeWrite(64, []byte{1, 2, 3, 4})
			topPort.Deliver(req)

			progress := modelingtest.Tick(rob)

			Expect(progress).To(BeTrue())
			Expect(rob.State.Transactions).To(HaveLen(1))
			Expect(rob.State.Transactions[0].IsRead).To(BeFalse())

			sent, _ := bottomPort.RetrieveOutgoing()
			Expect(sent).To(BeAssignableToTypeOf(messaging.Msg{Payload: memprotocol.WriteReq{}}))
			shadow := sent
			Expect(shadow.Payload.(memprotocol.WriteReq).Data).To(Equal([]byte{1, 2, 3, 4}))
			Expect(shadow.Dst).To(Equal(bottomUnitRemote))
		})

		It("stalls when the buffer is full", func() {
			spec := rob.Spec
			for i := 0; i < spec.BufferSize; i++ {
				rob.State.Transactions = append(rob.State.Transactions,
					transactionState{IsRead: true})
			}

			topPort.Deliver(makeRead(0))

			progress := modelingtest.Tick(rob)

			Expect(progress).To(BeFalse())
			Expect(rob.State.Transactions).To(HaveLen(spec.BufferSize))
			_, present0 := topPort.PeekIncoming()
			Expect(present0).To(BeTrue())
		})

		It("stalls when the bottom port is full", func() {
			topPort.Deliver(makeRead(0))

			// Fill the bottom outgoing buffer so Send fails.
			for i := 0; i < bottomBufSize; i++ {
				filler := messaging.Msg{Payload: memprotocol.ReadReq{Address: uint64(i)},
					ID:           sim.NewID(),
					Src:          bottomPort.AsRemote(),
					Dst:          bottomUnitRemote,
					TrafficClass: "memprotocol.ReadReq"}

				Expect(bottomPort.CanSend()).To(BeTrue())
				bottomPort.Send(filler)
			}

			progress := modelingtest.Tick(rob)

			Expect(progress).To(BeFalse())
			Expect(rob.State.Transactions).To(BeEmpty())
			_, present1 := topPort.PeekIncoming()
			Expect(present1).To(BeTrue())
		})

		It("panics on unsupported top-port traffic", func() {
			req := messaging.Msg{Payload: memcontrolprotocol.Req{Command: memcontrolprotocol.CmdFlush},
				ID:           sim.NewID(),
				Src:          topRemote,
				Dst:          topPort.AsRemote(),
				TrafficClass: "memcontrolprotocol.Req"}

			topPort.Deliver(req)

			Expect(func() { modelingtest.Tick(rob) }).To(Panic())
		})
	})

	Context("parse bottom", func() {
		It("attaches the bottom response to the matching transaction", func() {
			req := makeWrite(0, []byte{0xAA})
			topPort.Deliver(req)
			modelingtest.Tick(rob) // forward to bottom

			Expect(rob.State.Transactions).To(HaveLen(1))
			shadowID := rob.State.Transactions[0].ReqToBottomID

			rsp := messaging.Msg{Payload: memprotocol.WriteDoneRsp{},
				ID:           sim.NewID(),
				Src:          bottomUnitRemote,
				Dst:          bottomPort.AsRemote(),
				RspTo:        shadowID,
				TrafficClass: "memprotocol.WriteDoneRsp"}

			bottomPort.Deliver(rsp)

			modelingtest.Tick(rob)

			// The response is recorded on this tick; bottomUp will drain
			// the head on the following tick (hardware pipeline ordering).
			Expect(rob.State.Transactions).To(HaveLen(1))
			Expect(rob.State.Transactions[0].HasRsp).To(BeTrue())
		})

		It("ignores a response that does not match any transaction", func() {
			rsp := messaging.Msg{Payload: memprotocol.WriteDoneRsp{},
				ID:           sim.NewID(),
				Src:          bottomUnitRemote,
				Dst:          bottomPort.AsRemote(),
				RspTo:        999999,
				TrafficClass: "memprotocol.WriteDoneRsp"}

			bottomPort.Deliver(rsp)

			modelingtest.Tick(rob)

			_, present2 := bottomPort.PeekIncoming()
			Expect(present2).To(BeFalse())
		})

		It("drops unsupported bottom-port traffic", func() {
			req := messaging.Msg{Payload: memcontrolprotocol.Req{Command: memcontrolprotocol.CmdFlush},
				ID:           sim.NewID(),
				Src:          bottomUnitRemote,
				Dst:          bottomPort.AsRemote(),
				TrafficClass: "memcontrolprotocol.Req"}

			bottomPort.Deliver(req)

			progress := modelingtest.Tick(rob)

			Expect(progress).To(BeTrue())
			_, present3 := bottomPort.PeekIncoming()
			Expect(present3).To(BeFalse())
			Expect(rob.State.Transactions).To(BeEmpty())
		})
	})

	Context("bottom up", func() {
		It("forwards the head response to the top once it is ready", func() {
			req := makeRead(0)
			topPort.Deliver(req)
			modelingtest.Tick(rob)
			shadowSent, _ := bottomPort.RetrieveOutgoing()
			Expect(shadowSent).ToNot(BeNil())

			shadowID := rob.State.Transactions[0].ReqToBottomID

			rsp := messaging.Msg{Payload: memprotocol.DataReadyRsp{Data: []byte{0xDE, 0xAD}},
				ID:           sim.NewID(),
				Src:          bottomUnitRemote,
				Dst:          bottomPort.AsRemote(),
				RspTo:        shadowID,
				TrafficClass: "memprotocol.DataReadyRsp"}

			bottomPort.Deliver(rsp)

			modelingtest.Tick(rob) // parseBottom records the response
			modelingtest.Tick(rob) // bottomUp drains the head

			Expect(rob.State.Transactions).To(BeEmpty())

			topOut, _ := topPort.RetrieveOutgoing()
			Expect(topOut).To(BeAssignableToTypeOf(messaging.Msg{Payload: memprotocol.DataReadyRsp{}}))
			data := topOut
			Expect(data.Payload.(memprotocol.DataReadyRsp).Data).To(Equal([]byte{0xDE, 0xAD}))
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
			modelingtest.Tick(rob)
			Expect(rob.State.Transactions).To(HaveLen(2))
			_, present4 := bottomPort.RetrieveOutgoing()
			Expect(present4).To(BeTrue())
			_, present5 := bottomPort.RetrieveOutgoing()
			Expect(present5).To(BeTrue())
			shadow1 := rob.State.Transactions[0].ReqToBottomID
			shadow2 := rob.State.Transactions[1].ReqToBottomID

			// Deliver the second response first; the head must still wait
			// since its response has not arrived yet.
			rsp2 := messaging.Msg{Payload: memprotocol.DataReadyRsp{Data: []byte{0x22}},
				ID:           sim.NewID(),
				Src:          bottomUnitRemote,
				Dst:          bottomPort.AsRemote(),
				RspTo:        shadow2,
				TrafficClass: "memprotocol.DataReadyRsp"}

			bottomPort.Deliver(rsp2)

			modelingtest.Tick(rob) // parseBottom records rsp2
			modelingtest.Tick(rob) // bottomUp finds head not ready, makes no progress

			Expect(rob.State.Transactions).To(HaveLen(2))
			Expect(rob.State.Transactions[0].HasRsp).To(BeFalse())
			Expect(rob.State.Transactions[1].HasRsp).To(BeTrue())
			_, present6 := topPort.RetrieveOutgoing()
			Expect(present6).To(BeFalse())
			// Now deliver the response for the head; both should drain in
			// order on subsequent ticks.
			rsp1 := messaging.Msg{Payload: memprotocol.DataReadyRsp{Data: []byte{0x11}},
				ID:           sim.NewID(),
				Src:          bottomUnitRemote,
				Dst:          bottomPort.AsRemote(),
				RspTo:        shadow1,
				TrafficClass: "memprotocol.DataReadyRsp"}

			bottomPort.Deliver(rsp1)

			modelingtest.Tick(rob) // parseBottom records rsp1
			modelingtest.Tick(rob) // bottomUp drains both (NumReqPerCycle=2)

			Expect(rob.State.Transactions).To(BeEmpty())

			out1, _ := topPort.RetrieveOutgoing()
			out2, _ := topPort.RetrieveOutgoing()
			Expect(out1).To(BeAssignableToTypeOf(messaging.Msg{Payload: memprotocol.DataReadyRsp{}}))
			Expect(out2).To(BeAssignableToTypeOf(messaging.Msg{Payload: memprotocol.DataReadyRsp{}}))
			Expect(out1.Payload.(memprotocol.DataReadyRsp).Data).To(Equal([]byte{0x11}))
			Expect(out2.Payload.(memprotocol.DataReadyRsp).Data).To(Equal([]byte{0x22}))
			Expect(out1.RspTo).To(Equal(r1.ID))
			Expect(out2.RspTo).To(Equal(r2.ID))
		})

		It("stalls when the top port cannot accept the response", func() {
			req := makeRead(0)
			topPort.Deliver(req)
			modelingtest.Tick(rob)
			bottomPort.RetrieveOutgoing()

			shadowID := rob.State.Transactions[0].ReqToBottomID
			rsp := messaging.Msg{Payload: memprotocol.DataReadyRsp{Data: []byte{0x1}},
				ID:           sim.NewID(),
				Src:          bottomUnitRemote,
				Dst:          bottomPort.AsRemote(),
				RspTo:        shadowID,
				TrafficClass: "memprotocol.DataReadyRsp"}

			bottomPort.Deliver(rsp)

			modelingtest.Tick(rob) // parseBottom records the response

			// Fill the top outgoing buffer so the next bottomUp Send fails.
			for i := 0; i < topBufSize; i++ {
				filler := messaging.Msg{Payload: memprotocol.DataReadyRsp{Data: []byte{byte(i)}},
					ID:           sim.NewID(),
					Src:          topPort.AsRemote(),
					Dst:          topRemote,
					TrafficClass: "memprotocol.DataReadyRsp"}

				Expect(topPort.CanSend()).To(BeTrue())
				topPort.Send(filler)
			}

			modelingtest.Tick(rob)

			// The head should still be present and marked HasRsp because
			// bottomUp could not enqueue the outgoing response.
			Expect(rob.State.Transactions).To(HaveLen(1))
			Expect(rob.State.Transactions[0].HasRsp).To(BeTrue())
		})
	})

	Context("control", func() {
		It("drops in-flight transactions and pauses on CmdReset", func() {
			topPort.Deliver(makeRead(0))
			modelingtest.Tick(rob)
			bottomPort.RetrieveOutgoing()
			Expect(rob.State.Transactions).To(HaveLen(1))

			req := messaging.Msg{Payload: memcontrolprotocol.Req{
				Command: memcontrolprotocol.CmdReset,
			},
				ID:           sim.NewID(),
				Src:          messaging.RemotePort("Cmd"),
				Dst:          ctrlPort.AsRemote(),
				TrafficClass: "memcontrolprotocol.Req"}

			ctrlPort.Deliver(req)

			modelingtest.Tick(rob)

			Expect(rob.State.Transactions).To(BeEmpty())
			Expect(rob.State.ControlState).To(Equal(memcontrolprotocol.StateEnabled))

			ack, _ := ctrlPort.RetrieveOutgoing()
			Expect(ack).To(BeAssignableToTypeOf(messaging.Msg{Payload: memcontrolprotocol.Rsp{}}))
			rsp := ack
			Expect(rsp.Payload.(memcontrolprotocol.Rsp).Command).To(Equal(memcontrolprotocol.CmdReset))
			Expect(rsp.Payload.(memcontrolprotocol.Rsp).Success).To(BeTrue())
			Expect(rsp.RspTo).To(Equal(req.ID))
		})

		It("freezes the data pipeline while paused", func() {
			rob.State.ControlState = memcontrolprotocol.StatePaused
			topPort.Deliver(makeRead(0))

			progress := modelingtest.Tick(rob)

			Expect(progress).To(BeFalse())
			Expect(rob.State.Transactions).To(BeEmpty())
			_, present7 := topPort.PeekIncoming()
			Expect(present7).To(BeTrue())
		})

		It("resumes on CmdEnable, draining incoming traffic", func() {
			rob.State.ControlState = memcontrolprotocol.StatePaused

			// Stale traffic that should be cleared on resume.
			topPort.Deliver(makeRead(0))
			stray := messaging.Msg{Payload: memprotocol.DataReadyRsp{Data: []byte{0xFF}},
				ID:           sim.NewID(),
				Src:          bottomUnitRemote,
				Dst:          bottomPort.AsRemote(),
				RspTo:        0xDEAD,
				TrafficClass: "memprotocol.DataReadyRsp"}

			bottomPort.Deliver(stray)

			req := messaging.Msg{Payload: memcontrolprotocol.Req{Command: memcontrolprotocol.CmdEnable},
				ID:           sim.NewID(),
				Src:          messaging.RemotePort("Cmd"),
				Dst:          ctrlPort.AsRemote(),
				TrafficClass: "memcontrolprotocol.Req"}

			ctrlPort.Deliver(req)

			modelingtest.Tick(rob)

			Expect(rob.State.ControlState).To(Equal(memcontrolprotocol.StateEnabled))
			_, present8 := topPort.PeekIncoming()
			Expect(present8).To(BeFalse())
			_, present9 := bottomPort.PeekIncoming()
			Expect(present9).To(BeFalse())
			ack, _ := ctrlPort.RetrieveOutgoing()
			Expect(ack).To(BeAssignableToTypeOf(messaging.Msg{Payload: memcontrolprotocol.Rsp{}}))
			Expect(ack.Payload.(memcontrolprotocol.Rsp).Command).To(Equal(memcontrolprotocol.CmdEnable))
		})

		makeCtrlReq := func(cmd memcontrolprotocol.Command) messaging.Msg {
			req := messaging.Msg{Payload: memcontrolprotocol.Req{Command: cmd},
				ID:           sim.NewID(),
				Src:          messaging.RemotePort("Cmd"),
				Dst:          ctrlPort.AsRemote(),
				TrafficClass: "memcontrolprotocol.Req"}

			return req
		}

		It("acks Drain only after in-flight transactions retire", func() {
			const n = 2
			for i := range n {
				topPort.Deliver(makeRead(uint64(i * 0x100)))
			}
			modelingtest.Tick(rob) // forward both to the bottom (NumReqPerCycle=2)
			Expect(rob.State.Transactions).To(HaveLen(n))

			shadowIDs := make([]uint64, 0, n)
			for i := range rob.State.Transactions {
				shadowIDs = append(shadowIDs,
					rob.State.Transactions[i].ReqToBottomID)
			}
			for {
				if _, ok := bottomPort.RetrieveOutgoing(); !ok {
					break
				}
			}

			drain := makeCtrlReq(memcontrolprotocol.CmdDrain)
			ctrlPort.Deliver(drain)

			// While transactions are still in flight (no bottom responses
			// fed yet), Drain must stay pending and emit no ack.
			for range 5 {
				modelingtest.Tick(rob)
				Expect(rob.State.ControlState).To(Equal(memcontrolprotocol.StateDraining))
				_, present10 := ctrlPort.RetrieveOutgoing()
				Expect(present10).To(BeFalse())
			}
			Expect(rob.State.Transactions).To(HaveLen(n))

			// Now let the in-flight reads complete.
			for _, id := range shadowIDs {
				rsp := messaging.Msg{Payload: memprotocol.DataReadyRsp{Data: []byte{0x1}},
					ID:           sim.NewID(),
					Src:          bottomUnitRemote,
					Dst:          bottomPort.AsRemote(),
					RspTo:        id,
					TrafficClass: "memprotocol.DataReadyRsp"}

				bottomPort.Deliver(rsp)
			}

			completed := 0
			var drainRsp messaging.Msg
			drainFound := false
			for i := 0; i < 64 && !drainFound; i++ {
				modelingtest.Tick(rob)
				for {
					out, ok := topPort.RetrieveOutgoing()
					if !ok {
						break
					}
					if _, ok := out.Payload.(memprotocol.DataReadyRsp); ok {
						completed++
					}
				}
				if out, ok := ctrlPort.RetrieveOutgoing(); ok {
					if rsp, ok := out.Payload.(memcontrolprotocol.Rsp); ok && rsp.
						Command == memcontrolprotocol.CmdDrain {
						drainRsp = out

						drainFound = true
					}

				}
			}

			Expect(drainFound).To(BeTrue())
			Expect(drainRsp.Payload.(memcontrolprotocol.Rsp).Success).To(BeTrue())
			Expect(drainRsp.RspTo).To(Equal(drain.ID))
			// All in-flight reads finished before the async Drain ack.
			Expect(completed).To(Equal(n))
			Expect(rob.State.Transactions).To(BeEmpty())
			Expect(rob.State.ControlState).To(Equal(memcontrolprotocol.StatePaused))
		})

		DescribeTable("Reset wipes in-flight transactions from any state",
			func(startState memcontrolprotocol.State) {
				topPort.Deliver(makeRead(0))
				modelingtest.Tick(rob)
				Expect(rob.State.Transactions).To(HaveLen(1))

				rob.State.ControlState = startState

				reset := makeCtrlReq(memcontrolprotocol.CmdReset)
				ctrlPort.Deliver(reset)

				var rsp messaging.Msg
				found := false
				for i := 0; i < 64 && !found; i++ {
					modelingtest.Tick(rob)
					if out, ok := ctrlPort.RetrieveOutgoing(); ok {
						_, found = out.Payload.(memcontrolprotocol.Rsp)
						rsp = out
					}
				}

				Expect(found).To(BeTrue())
				Expect(rsp.Payload.(memcontrolprotocol.Rsp).Command).To(Equal(memcontrolprotocol.CmdReset))
				Expect(rsp.Payload.(memcontrolprotocol.Rsp).Success).To(BeTrue())
				Expect(rsp.RspTo).To(Equal(reset.ID))
				Expect(rob.State.Transactions).To(BeEmpty())
				Expect(rob.State.ControlState).To(Equal(memcontrolprotocol.StateEnabled))
			},
			Entry("from Enabled", memcontrolprotocol.StateEnabled),
			Entry("from Paused", memcontrolprotocol.StatePaused),
			// The draining case is covered separately by the test below, since
			// control commands are serialized: a Reset queued behind an
			// in-progress Drain is only serviced after the Drain acks.
		)

		It("completes a pending Drain before servicing a queued Reset", func() {
			// Draining and already quiescent (no in-flight transactions):
			// completePendingDrain acks the Drain. Control commands are
			// serialized with no preemption, so a Reset queued behind the
			// drain is serviced only after the Drain acks.
			rob.State.ControlState = memcontrolprotocol.StateDraining
			rob.State.CurrentCmdID = 999
			rob.State.CurrentCmdSrc = messaging.RemotePort("Drainer")
			rob.State.Transactions = nil

			reset := makeCtrlReq(memcontrolprotocol.CmdReset)
			ctrlPort.Deliver(reset)

			var rsps []messaging.Msg
			for range 16 {
				modelingtest.Tick(rob)
				for {
					out, ok := ctrlPort.RetrieveOutgoing()
					if !ok {
						break
					}
					if _, ok := out.Payload.(memcontrolprotocol.Rsp); ok {

						rsps = append(rsps, out)
					}
				}
			}

			Expect(rsps).To(HaveLen(2))
			Expect(rsps[0].Payload.(memcontrolprotocol.Rsp).Command).To(Equal(memcontrolprotocol.CmdDrain))
			Expect(rsps[0].RspTo).To(Equal(uint64(999)))
			Expect(rsps[1].Payload.(memcontrolprotocol.Rsp).Command).To(Equal(memcontrolprotocol.CmdReset))
			Expect(rsps[1].RspTo).To(Equal(reset.ID))
			Expect(rob.State.ControlState).To(Equal(memcontrolprotocol.StateEnabled))
		})
	})
})
