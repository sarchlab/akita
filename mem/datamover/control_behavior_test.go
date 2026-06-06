package datamover

import (
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/sarchlab/akita/v5/mem"
	"github.com/sarchlab/akita/v5/mem/control"
	"github.com/sarchlab/akita/v5/messaging"
	"github.com/sarchlab/akita/v5/modeling"
	"github.com/sarchlab/akita/v5/timing"
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
		dataMover    *modeling.Component[Spec, State, modeling.None]
		topPort      messaging.Port
		ctrlPort     messaging.Port
		insidePort   messaging.Port
		outsidePort  messaging.Port
		insideRemote messaging.RemotePort
	)

	build := func() {
		spec := DefaultSpec()
		spec.BufferSize = 2048
		spec.InsideByteGranularity = 64
		spec.OutsideByteGranularity = 64
		spec.CtrlPortBufferSize = 1024
		spec.InsidePortBufferSize = 64
		spec.OutsidePortBufferSize = 64

		insideRemote = messaging.RemotePort("InsideMem")
		dataMover = MakeBuilder().
			WithRegistrar(modeling.NewStandaloneRegistrar(engine)).
			WithSpec(spec).
			WithResources(Resources{
				InsideMapper: &mem.SinglePortMapper{Port: insideRemote},
				OutsideMapper: &mem.SinglePortMapper{
					Port: messaging.RemotePort("OutsideMem"),
				},
			}).
			Build("DataMover")

		topPort = dataMover.GetPortByName("Top")
		ctrlPort = dataMover.GetPortByName("Control")
		insidePort = dataMover.GetPortByName("Inside")
		outsidePort = dataMover.GetPortByName("Outside")
		for _, p := range []messaging.Port{
			topPort, ctrlPort, insidePort, outsidePort,
		} {
			(&ccNoopConn{}).PlugIn(p)
		}
	}

	// makeMove builds a 64-byte outside->inside transfer, the minimal move
	// (one read on Outside, one write on Inside).
	makeMove := func() DataMoveRequest {
		req := DataMoveRequest{}
		req.ID = timing.GetIDGenerator().Generate()
		req.Src = messaging.RemotePort("Agent")
		req.Dst = topPort.AsRemote()
		req.SrcAddress = 0
		req.SrcSide = "outside"
		req.DstAddress = 0
		req.DstSide = "inside"
		req.ByteSize = 64
		req.TrafficClass = "datamover.DataMoveRequest"
		return req
	}

	makeCtrlReq := func(cmd mem.ControlCommand) mem.ControlReq {
		req := mem.ControlReq{Command: cmd}
		req.ID = timing.GetIDGenerator().Generate()
		req.Src = messaging.RemotePort("Cmd")
		req.Dst = ctrlPort.AsRemote()
		req.TrafficClass = "mem.ControlReq"
		return req
	}

	answerRead := func(port messaging.Port, read mem.ReadReq) {
		rsp := mem.DataReadyRsp{Data: make([]byte, int(read.AccessByteSize))}
		rsp.ID = timing.GetIDGenerator().Generate()
		rsp.Src = read.Dst
		rsp.Dst = port.AsRemote()
		rsp.RspTo = read.ID
		rsp.TrafficClass = "mem.DataReadyRsp"
		port.Deliver(rsp)
	}

	answerWrite := func(port messaging.Port, write mem.WriteReq) {
		rsp := mem.WriteDoneRsp{}
		rsp.ID = timing.GetIDGenerator().Generate()
		rsp.Src = write.Dst
		rsp.Dst = port.AsRemote()
		rsp.RspTo = write.ID
		rsp.TrafficClass = "mem.WriteDoneRsp"
		port.Deliver(rsp)
	}

	// startMove delivers a move and ticks until it is active and the first
	// Outside read has been issued, returning that read.
	startMove := func() (mem.ReadReq, bool) {
		topPort.Deliver(makeMove())

		var read mem.ReadReq
		gotRead := false
		for i := 0; i < 64 && !gotRead; i++ {
			dataMover.Tick()
			if out := outsidePort.RetrieveOutgoing(); out != nil {
				read, gotRead = out.(mem.ReadReq)
			}
		}
		return read, gotRead
	}

	BeforeEach(func() {
		engine = timing.NewSerialEngine()
		build()
	})

	It("acks Drain only after the in-flight move completes", func() {
		read, gotRead := startMove()
		Expect(gotRead).To(BeTrue())
		Expect(dataMover.State.CurrentTransaction.Active).To(BeTrue())

		drain := makeCtrlReq(mem.CmdDrain)
		ctrlPort.Deliver(drain)

		// The move is stuck waiting for its read response, so Drain must
		// stay pending and emit no ack.
		for range 5 {
			dataMover.Tick()
			Expect(dataMover.State.ControlState).
				To(Equal(control.StateDraining))
			Expect(dataMover.State.CurrentTransaction.Active).To(BeTrue())
			Expect(ctrlPort.RetrieveOutgoing()).To(BeNil())
		}

		// Let the move finish: answer the read, then the write it triggers.
		answerRead(outsidePort, read)

		var drainRsp mem.ControlRsp
		gotDrainRsp := false
		var write mem.WriteReq
		gotWrite := false
		moveDone := false
		for i := 0; i < 256 && !gotDrainRsp; i++ {
			dataMover.Tick()
			if !gotWrite {
				if out := insidePort.RetrieveOutgoing(); out != nil {
					if w, ok := out.(mem.WriteReq); ok {
						write = w
						gotWrite = true
						answerWrite(insidePort, write)
					}
				}
			}
			if out := topPort.RetrieveOutgoing(); out != nil {
				if _, ok := out.(DataMoveResponse); ok {
					moveDone = true
				}
			}
			if out := ctrlPort.RetrieveOutgoing(); out != nil {
				if rsp, ok := out.(mem.ControlRsp); ok &&
					rsp.Command == mem.CmdDrain {
					drainRsp = rsp
					gotDrainRsp = true
				}
			}
		}

		Expect(gotWrite).To(BeTrue())
		Expect(gotDrainRsp).To(BeTrue())
		Expect(drainRsp.Success).To(BeTrue())
		Expect(drainRsp.RspTo).To(Equal(drain.ID))
		// The move completed (response emitted) before the async Drain ack.
		Expect(moveDone).To(BeTrue())
		Expect(dataMover.State.CurrentTransaction.Active).To(BeFalse())
		Expect(dataMover.State.ControlState).To(Equal(control.StatePaused))
	})

	It("acks Drain only after outstanding destination writes are acked", func() {
		read, gotRead := startMove()
		Expect(gotRead).To(BeTrue())

		// Answer the read so the data mover issues the destination write.
		answerRead(outsidePort, read)
		var write mem.WriteReq
		gotWrite := false
		for i := 0; i < 64 && !gotWrite; i++ {
			dataMover.Tick()
			if out := insidePort.RetrieveOutgoing(); out != nil {
				write, gotWrite = out.(mem.WriteReq)
			}
		}
		Expect(gotWrite).To(BeTrue())
		Expect(dataMover.State.CurrentTransaction.Active).To(BeTrue())
		Expect(dataMover.State.CurrentTransaction.PendingWrite).ToNot(BeEmpty())

		// Drain while the write's ack is still outstanding: the move is not
		// complete, so Drain must stay pending and emit no move response.
		drain := makeCtrlReq(mem.CmdDrain)
		ctrlPort.Deliver(drain)
		for range 8 {
			dataMover.Tick()
			Expect(topPort.RetrieveOutgoing()).To(BeNil())
			Expect(ctrlPort.RetrieveOutgoing()).To(BeNil())
			Expect(dataMover.State.CurrentTransaction.Active).To(BeTrue())
		}

		// Ack the write; only now may the move finish and the Drain ack.
		answerWrite(insidePort, write)
		moveDone := false
		gotDrainRsp := false
		var drainRsp mem.ControlRsp
		for i := 0; i < 256 && !gotDrainRsp; i++ {
			dataMover.Tick()
			if out := topPort.RetrieveOutgoing(); out != nil {
				if _, ok := out.(DataMoveResponse); ok {
					moveDone = true
				}
			}
			if out := ctrlPort.RetrieveOutgoing(); out != nil {
				if rsp, ok := out.(mem.ControlRsp); ok &&
					rsp.Command == mem.CmdDrain {
					drainRsp = rsp
					gotDrainRsp = true
				}
			}
		}

		Expect(moveDone).To(BeTrue())
		Expect(gotDrainRsp).To(BeTrue())
		Expect(drainRsp.Success).To(BeTrue())
		Expect(drainRsp.RspTo).To(Equal(drain.ID))
		Expect(dataMover.State.CurrentTransaction.Active).To(BeFalse())
		Expect(dataMover.State.ControlState).To(Equal(control.StatePaused))
	})

	It("drops a stale memory ack that arrives after Reset", func() {
		// Move 1 issues an Outside read (readA), then is reset mid-flight.
		readA, ok := startMove()
		Expect(ok).To(BeTrue())

		reset := makeCtrlReq(mem.CmdReset)
		ctrlPort.Deliver(reset)
		acked := false
		for i := 0; i < 64 && !acked; i++ {
			dataMover.Tick()
			if out := ctrlPort.RetrieveOutgoing(); out != nil {
				if r, ok := out.(mem.ControlRsp); ok &&
					r.Command == mem.CmdReset {
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
			dataMover.Tick()
		}
		Expect(dataMover.State.CurrentTransaction.Active).To(BeTrue())

		// Move 2 completes once its own read and write are answered.
		answerRead(outsidePort, readB)
		gotWrite := false
		moveDone := false
		for i := 0; i < 256 && !moveDone; i++ {
			dataMover.Tick()
			if !gotWrite {
				if out := insidePort.RetrieveOutgoing(); out != nil {
					if w, ok := out.(mem.WriteReq); ok {
						gotWrite = true
						answerWrite(insidePort, w)
					}
				}
			}
			if out := topPort.RetrieveOutgoing(); out != nil {
				if _, ok := out.(DataMoveResponse); ok {
					moveDone = true
				}
			}
		}
		Expect(moveDone).To(BeTrue())
	})

	It("freezes incoming move requests while paused", func() {
		dataMover.State.ControlState = control.StatePaused
		topPort.Deliver(makeMove())

		for range 5 {
			dataMover.Tick()
		}

		Expect(topPort.PeekIncoming()).ToNot(BeNil())
		Expect(dataMover.State.CurrentTransaction.Active).To(BeFalse())
		Expect(outsidePort.RetrieveOutgoing()).To(BeNil())
	})

	DescribeTable("Reset wipes the in-flight move from any control state",
		func(startState control.State) {
			_, gotRead := startMove()
			Expect(gotRead).To(BeTrue())
			Expect(dataMover.State.CurrentTransaction.Active).To(BeTrue())

			dataMover.State.ControlState = startState

			reset := makeCtrlReq(mem.CmdReset)
			ctrlPort.Deliver(reset)

			var rsp mem.ControlRsp
			gotRsp := false
			for i := 0; i < 64 && !gotRsp; i++ {
				dataMover.Tick()
				if out := ctrlPort.RetrieveOutgoing(); out != nil {
					rsp, gotRsp = out.(mem.ControlRsp)
				}
			}

			Expect(gotRsp).To(BeTrue())
			Expect(rsp.Command).To(Equal(mem.CmdReset))
			Expect(rsp.Success).To(BeTrue())
			Expect(rsp.RspTo).To(Equal(reset.ID))
			Expect(dataMover.State.CurrentTransaction.Active).To(BeFalse())
			Expect(dataMover.State.CurrentTransaction.PendingRead).To(BeEmpty())
			Expect(dataMover.State.CurrentTransaction.PendingWrite).
				To(BeEmpty())
			Expect(dataMover.State.ControlState).To(Equal(control.StateEnabled))
		},
		Entry("from Enabled", control.StateEnabled),
		Entry("from Paused", control.StatePaused),
		// The Draining case is covered separately below: under strict
		// serialization a Reset queued behind an in-progress Drain is not
		// serviced until the Drain acks, so it needs its own scenario.
	)

	It("completes a pending Drain before servicing a queued Reset", func() {
		// Draining and already quiescent (no active transfer):
		// completePendingDrain acks the Drain. Control commands are serialized
		// with no preemption, so a Reset queued behind the drain is serviced
		// only after the Drain acks.
		dataMover.State.ControlState = control.StateDraining
		dataMover.State.CurrentCmdID = 999
		dataMover.State.CurrentCmdSrc = messaging.RemotePort("Drainer")

		reset := makeCtrlReq(mem.CmdReset)
		ctrlPort.Deliver(reset)

		var rsps []mem.ControlRsp
		for range 16 {
			dataMover.Tick()
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
		Expect(dataMover.State.ControlState).To(Equal(control.StateEnabled))
	})
})
