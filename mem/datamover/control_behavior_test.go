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
	makeMove := func() *DataMoveRequest {
		req := &DataMoveRequest{}
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

	makeCtrlReq := func(cmd mem.ControlCommand) *mem.ControlReq {
		req := &mem.ControlReq{Command: cmd}
		req.ID = timing.GetIDGenerator().Generate()
		req.Src = messaging.RemotePort("Cmd")
		req.Dst = ctrlPort.AsRemote()
		req.TrafficClass = "mem.ControlReq"
		return req
	}

	answerRead := func(port messaging.Port, read *mem.ReadReq) {
		rsp := &mem.DataReadyRsp{Data: make([]byte, int(read.AccessByteSize))}
		rsp.ID = timing.GetIDGenerator().Generate()
		rsp.Src = read.Dst
		rsp.Dst = port.AsRemote()
		rsp.RspTo = read.ID
		rsp.TrafficClass = "mem.DataReadyRsp"
		port.Deliver(rsp)
	}

	answerWrite := func(port messaging.Port, write *mem.WriteReq) {
		rsp := &mem.WriteDoneRsp{}
		rsp.ID = timing.GetIDGenerator().Generate()
		rsp.Src = write.Dst
		rsp.Dst = port.AsRemote()
		rsp.RspTo = write.ID
		rsp.TrafficClass = "mem.WriteDoneRsp"
		port.Deliver(rsp)
	}

	// startMove delivers a move and ticks until it is active and the first
	// Outside read has been issued, returning that read.
	startMove := func() *mem.ReadReq {
		topPort.Deliver(makeMove())

		var read *mem.ReadReq
		for i := 0; i < 64 && read == nil; i++ {
			dataMover.Tick()
			if out := outsidePort.RetrieveOutgoing(); out != nil {
				read, _ = out.(*mem.ReadReq)
			}
		}
		return read
	}

	BeforeEach(func() {
		engine = timing.NewSerialEngine()
		build()
	})

	It("acks Drain only after the in-flight move completes", func() {
		read := startMove()
		Expect(read).ToNot(BeNil())
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

		var drainRsp *mem.ControlRsp
		var write *mem.WriteReq
		moveDone := false
		for i := 0; i < 256 && drainRsp == nil; i++ {
			dataMover.Tick()
			if write == nil {
				if out := insidePort.RetrieveOutgoing(); out != nil {
					if w, ok := out.(*mem.WriteReq); ok {
						write = w
						answerWrite(insidePort, write)
					}
				}
			}
			if out := topPort.RetrieveOutgoing(); out != nil {
				if _, ok := out.(*DataMoveResponse); ok {
					moveDone = true
				}
			}
			if out := ctrlPort.RetrieveOutgoing(); out != nil {
				if rsp, ok := out.(*mem.ControlRsp); ok &&
					rsp.Command == mem.CmdDrain {
					drainRsp = rsp
				}
			}
		}

		Expect(write).ToNot(BeNil())
		Expect(drainRsp).ToNot(BeNil())
		Expect(drainRsp.Success).To(BeTrue())
		Expect(drainRsp.RspTo).To(Equal(drain.ID))
		// The move completed (response emitted) before the async Drain ack.
		Expect(moveDone).To(BeTrue())
		Expect(dataMover.State.CurrentTransaction.Active).To(BeFalse())
		Expect(dataMover.State.ControlState).To(Equal(control.StatePaused))
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
			read := startMove()
			Expect(read).ToNot(BeNil())
			Expect(dataMover.State.CurrentTransaction.Active).To(BeTrue())

			dataMover.State.ControlState = startState

			reset := makeCtrlReq(mem.CmdReset)
			ctrlPort.Deliver(reset)

			var rsp *mem.ControlRsp
			for i := 0; i < 64 && rsp == nil; i++ {
				dataMover.Tick()
				if out := ctrlPort.RetrieveOutgoing(); out != nil {
					rsp, _ = out.(*mem.ControlRsp)
				}
			}

			Expect(rsp).ToNot(BeNil())
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
		Entry("from Draining", control.StateDraining),
	)
})
