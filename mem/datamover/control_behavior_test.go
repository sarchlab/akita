package datamover

import (
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/sarchlab/akita/v5/mem"
	"github.com/sarchlab/akita/v5/mem/datamoverprotocol"
	"github.com/sarchlab/akita/v5/mem/memcontrolprotocol"
	"github.com/sarchlab/akita/v5/mem/memprotocol"
	"github.com/sarchlab/akita/v5/sim/messaging"
	"github.com/sarchlab/akita/v5/sim/modeling"
	"github.com/sarchlab/akita/v5/sim/modeling/modelingtest"
	"github.com/sarchlab/akita/v5/sim/timing"
)

// This file holds Layer-2 control-behavior tests: it drives the data mover
// one tick at a time and answers its Inside/Outside memory traffic by hand
// so the universal verbs can be observed deterministically (Drain only acks
// once the in-flight move finishes, Pause freezes intake, Reset wipes the
// move from any state). The protocol surface is covered separately in
// control_contract_test.go.
var _ = Describe("DataMover control behavior", func() {
	var (
		engine       timing.Engine
		sim          timing.Simulation
		dataMover    *Comp
		topPort      messaging.Port
		ctrlPort     messaging.Port
		insidePort   messaging.Port
		outsidePort  messaging.Port
		insideRemote messaging.RemotePort
	)

	build := func() {
		spec := Definition.DefaultSpec
		spec.BufferSize = 2048
		spec.InsideByteGranularity = 64
		spec.OutsideByteGranularity = 64

		insideRemote = messaging.RemotePort("InsideMem")
		dataMover = Definition.Builder().
			WithSimulation(sim).
			WithSpec(spec).
			WithResources(Resources{
				InsideMapper: &mem.SinglePortMapper{Port: insideRemote},
				OutsideMapper: &mem.SinglePortMapper{
					Port: messaging.RemotePort("OutsideMem"),
				},
			}).
			WithPorts(makePorts("DataMover", 16, 64, 64, 1024)).
			Build("DataMover")

		topPort = dataMover.Ports.Top
		insidePort = dataMover.Ports.Inside
		outsidePort = dataMover.Ports.Outside
		ctrlPort = dataMover.Ports.Control
		for _, p := range []messaging.Port{
			topPort, ctrlPort, insidePort, outsidePort,
		} {
			(&ccNoopConn{}).PlugIn(p)
		}
	}

	// makeMove builds a 64-byte outside->inside transfer, the minimal move
	// (one read on Outside, one write on Inside).
	makeMove := func() messaging.Msg {
		req := messaging.Msg{Payload: datamoverprotocol.DataMoveReq{
			SrcAddress: 0,
			SrcSide:    "outside",
			DstAddress: 0,
			DstSide:    "inside",
			ByteSize:   64},
			ID:  sim.NewID(),
			Src: messaging.RemotePort("Agent"),
			Dst: topPort.AsRemote(),

			TrafficClass: "datamoverprotocol.DataMoveReq"}

		return req
	}

	makeCtrlReq := func(cmd memcontrolprotocol.Command) messaging.Msg {
		req := messaging.Msg{Payload: memcontrolprotocol.Req{Command: cmd},
			ID:           sim.NewID(),
			Src:          messaging.RemotePort("Cmd"),
			Dst:          ctrlPort.AsRemote(),
			TrafficClass: "memcontrolprotocol.Req"}

		return req
	}

	answerRead := func(port messaging.Port, read messaging.Msg) {
		data := make([]byte, int(read.Payload.(memprotocol.ReadReq).AccessByteSize))
		rsp := messaging.Msg{Payload: memprotocol.DataReadyRsp{Data: data},
			ID:           sim.NewID(),
			Src:          read.Dst,
			Dst:          port.AsRemote(),
			RspTo:        read.ID,
			TrafficClass: "memprotocol.DataReadyRsp"}

		port.Deliver(rsp)
	}

	answerWrite := func(port messaging.Port, write messaging.Msg) {
		rsp := messaging.Msg{Payload: memprotocol.WriteDoneRsp{},
			ID:           sim.NewID(),
			Src:          write.Dst,
			Dst:          port.AsRemote(),
			RspTo:        write.ID,
			TrafficClass: "memprotocol.WriteDoneRsp"}

		port.Deliver(rsp)
	}

	// startMove delivers a move and ticks until it is active and the first
	// Outside read has been issued, returning that read.
	startMove := func() (messaging.Msg, bool) {
		topPort.Deliver(makeMove())

		var read messaging.Msg
		gotRead := false
		for i := 0; i < 64 && !gotRead; i++ {
			modelingtest.Tick(dataMover)
			if out, ok := outsidePort.RetrieveOutgoing(); ok {
				_, gotRead = out.Payload.(memprotocol.ReadReq)
				read = out
			}
		}
		return read, gotRead
	}

	BeforeEach(func() {
		engine = timing.NewSerialEngine()
		sim = modeling.NewStandaloneSimulation(engine)
		build()
	})

	It("acks Drain only after the in-flight move completes", func() {
		read, gotRead := startMove()
		Expect(gotRead).To(BeTrue())
		Expect(dataMover.State.CurrentTransaction.Active).To(BeTrue())

		drain := makeCtrlReq(memcontrolprotocol.CmdDrain)
		ctrlPort.Deliver(drain)

		// The move is stuck waiting for its read response, so Drain must
		// stay pending and emit no ack.
		for range 5 {
			modelingtest.Tick(dataMover)
			Expect(dataMover.State.ControlState).
				To(Equal(memcontrolprotocol.StateDraining))
			Expect(dataMover.State.CurrentTransaction.Active).To(BeTrue())
			_, present0 := ctrlPort.RetrieveOutgoing()
			Expect(present0).To(BeFalse())
		}

		// Let the move finish: answer the read, then the write it triggers.
		answerRead(outsidePort, read)

		var drainRsp messaging.Msg
		gotDrainRsp := false
		var write messaging.Msg
		gotWrite := false
		moveDone := false
		for i := 0; i < 256 && !gotDrainRsp; i++ {
			modelingtest.Tick(dataMover)
			if !gotWrite {
				if out, ok := insidePort.RetrieveOutgoing(); ok {
					if _, ok := out.Payload.(memprotocol.WriteReq); ok {

						write = out

						gotWrite = true
						answerWrite(insidePort, write)
					}
				}
			}
			if out, ok := topPort.RetrieveOutgoing(); ok {
				if _, ok := out.Payload.(datamoverprotocol.DataMoveRsp); ok {
					moveDone = true
				}
			}
			if out, ok := ctrlPort.RetrieveOutgoing(); ok {
				if rsp, ok := out.Payload.(memcontrolprotocol.Rsp); ok && rsp.
					Command == memcontrolprotocol.CmdDrain {
					drainRsp = out

					gotDrainRsp = true
				}

			}
		}

		Expect(gotWrite).To(BeTrue())
		Expect(gotDrainRsp).To(BeTrue())
		Expect(drainRsp.Payload.(memcontrolprotocol.Rsp).Success).To(BeTrue())
		Expect(drainRsp.RspTo).To(Equal(drain.ID))
		// The move completed (response emitted) before the async Drain ack.
		Expect(moveDone).To(BeTrue())
		Expect(dataMover.State.CurrentTransaction.Active).To(BeFalse())
		Expect(dataMover.State.ControlState).To(Equal(memcontrolprotocol.StatePaused))
	})

	It("acks Drain only after outstanding destination writes are acked", func() {
		read, gotRead := startMove()
		Expect(gotRead).To(BeTrue())

		// Answer the read so the data mover issues the destination write.
		answerRead(outsidePort, read)
		var write messaging.Msg
		gotWrite := false
		for i := 0; i < 64 && !gotWrite; i++ {
			modelingtest.Tick(dataMover)
			if out, ok := insidePort.RetrieveOutgoing(); ok {
				_, gotWrite = out.Payload.(memprotocol.WriteReq)
				write = out
			}
		}
		Expect(gotWrite).To(BeTrue())
		Expect(dataMover.State.CurrentTransaction.Active).To(BeTrue())
		Expect(dataMover.State.CurrentTransaction.PendingWrite).ToNot(BeEmpty())

		// Drain while the write's ack is still outstanding: the move is not
		// complete, so Drain must stay pending and emit no move response.
		drain := makeCtrlReq(memcontrolprotocol.CmdDrain)
		ctrlPort.Deliver(drain)
		for range 8 {
			modelingtest.Tick(dataMover)
			_, present1 := topPort.RetrieveOutgoing()
			Expect(present1).To(BeFalse())
			_, present2 := ctrlPort.RetrieveOutgoing()
			Expect(present2).To(BeFalse())
			Expect(dataMover.State.CurrentTransaction.Active).To(BeTrue())
		}

		// Ack the write; only now may the move finish and the Drain ack.
		answerWrite(insidePort, write)
		moveDone := false
		gotDrainRsp := false
		var drainRsp messaging.Msg
		for i := 0; i < 256 && !gotDrainRsp; i++ {
			modelingtest.Tick(dataMover)
			if out, ok := topPort.RetrieveOutgoing(); ok {
				if _, ok := out.Payload.(datamoverprotocol.DataMoveRsp); ok {
					moveDone = true
				}
			}
			if out, ok := ctrlPort.RetrieveOutgoing(); ok {
				if rsp, ok := out.Payload.(memcontrolprotocol.Rsp); ok && rsp.
					Command == memcontrolprotocol.CmdDrain {
					drainRsp = out

					gotDrainRsp = true
				}

			}
		}

		Expect(moveDone).To(BeTrue())
		Expect(gotDrainRsp).To(BeTrue())
		Expect(drainRsp.Payload.(memcontrolprotocol.Rsp).Success).To(BeTrue())
		Expect(drainRsp.RspTo).To(Equal(drain.ID))
		Expect(dataMover.State.CurrentTransaction.Active).To(BeFalse())
		Expect(dataMover.State.ControlState).To(Equal(memcontrolprotocol.StatePaused))
	})

	It("drops a stale memory ack that arrives after Reset", func() {
		// Move 1 issues an Outside read (readA), then is reset mid-flight.
		readA, ok := startMove()
		Expect(ok).To(BeTrue())

		reset := makeCtrlReq(memcontrolprotocol.CmdReset)
		ctrlPort.Deliver(reset)
		acked := false
		for i := 0; i < 64 && !acked; i++ {
			modelingtest.Tick(dataMover)
			if out, ok := ctrlPort.RetrieveOutgoing(); ok {
				if r, ok := out.Payload.(memcontrolprotocol.Rsp); ok && r.
					Command == memcontrolprotocol.CmdReset {
					acked = true
				}

			}
		}
		Expect(acked).To(BeTrue())

		// Move 2 issues its own Outside read (readB) with a different ID.
		readB, ok := startMove()
		Expect(ok).To(BeTrue())
		Expect(readB.ID).ToNot(Equal(readA.ID))

		// The stale response for readA arrives. It must be dropped (no panic),
		// leaving move 2 in flight.
		answerRead(outsidePort, readA)
		for range 8 {
			modelingtest.Tick(dataMover)
		}
		Expect(dataMover.State.CurrentTransaction.Active).To(BeTrue())

		// Move 2 completes once its own read and write are answered.
		answerRead(outsidePort, readB)
		gotWrite := false
		moveDone := false
		for i := 0; i < 256 && !moveDone; i++ {
			modelingtest.Tick(dataMover)
			if !gotWrite {
				if out, ok := insidePort.RetrieveOutgoing(); ok {
					if _, ok := out.Payload.(memprotocol.WriteReq); ok {

						gotWrite = true
						answerWrite(insidePort, out)
					}
				}
			}
			if out, ok := topPort.RetrieveOutgoing(); ok {
				if _, ok := out.Payload.(datamoverprotocol.DataMoveRsp); ok {
					moveDone = true
				}
			}
		}
		Expect(moveDone).To(BeTrue())
	})

	It("freezes incoming move requests while paused", func() {
		dataMover.State.ControlState = memcontrolprotocol.StatePaused
		topPort.Deliver(makeMove())

		for range 5 {
			modelingtest.Tick(dataMover)
		}

		_, present3 := topPort.PeekIncoming()
		Expect(present3).To(BeTrue())
		Expect(dataMover.State.CurrentTransaction.Active).To(BeFalse())
		_, present4 := outsidePort.RetrieveOutgoing()
		Expect(present4).To(BeFalse())
	})

	DescribeTable("Reset wipes the in-flight move from any control state",
		func(startState memcontrolprotocol.State) {
			_, gotRead := startMove()
			Expect(gotRead).To(BeTrue())
			Expect(dataMover.State.CurrentTransaction.Active).To(BeTrue())

			dataMover.State.ControlState = startState

			reset := makeCtrlReq(memcontrolprotocol.CmdReset)
			ctrlPort.Deliver(reset)

			var rsp messaging.Msg
			gotRsp := false
			for i := 0; i < 64 && !gotRsp; i++ {
				modelingtest.Tick(dataMover)
				if out, ok := ctrlPort.RetrieveOutgoing(); ok {
					_, gotRsp = out.Payload.(memcontrolprotocol.Rsp)
					rsp = out
				}
			}

			Expect(gotRsp).To(BeTrue())
			Expect(rsp.Payload.(memcontrolprotocol.Rsp).Command).To(Equal(memcontrolprotocol.CmdReset))
			Expect(rsp.Payload.(memcontrolprotocol.Rsp).Success).To(BeTrue())
			Expect(rsp.RspTo).To(Equal(reset.ID))
			Expect(dataMover.State.CurrentTransaction.Active).To(BeFalse())
			Expect(dataMover.State.CurrentTransaction.PendingRead).To(BeEmpty())
			Expect(dataMover.State.CurrentTransaction.PendingWrite).
				To(BeEmpty())
			Expect(dataMover.State.ControlState).To(Equal(memcontrolprotocol.StateEnabled))
		},
		Entry("from Enabled", memcontrolprotocol.StateEnabled),
		Entry("from Paused", memcontrolprotocol.StatePaused),
		// The Draining case is covered separately below: under strict
		// serialization a Reset queued behind an in-progress Drain is not
		// serviced until the Drain acks, so it needs its own scenario.
	)

	It("completes a pending Drain before servicing a queued Reset", func() {
		// Draining and already quiescent (no active transfer):
		// completePendingDrain acks the Drain. Control commands are serialized
		// with no preemption, so a Reset queued behind the drain is serviced
		// only after the Drain acks.
		dataMover.State.ControlState = memcontrolprotocol.StateDraining
		dataMover.State.CurrentCmdID = 999
		dataMover.State.CurrentCmdSrc = messaging.RemotePort("Drainer")

		reset := makeCtrlReq(memcontrolprotocol.CmdReset)
		ctrlPort.Deliver(reset)

		var rsps []messaging.Msg
		for range 16 {
			modelingtest.Tick(dataMover)
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
		Expect(dataMover.State.ControlState).To(Equal(memcontrolprotocol.StateEnabled))
	})
})
