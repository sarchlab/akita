package idealmemcontroller

import (
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/sarchlab/akita/v5/mem"
	"github.com/sarchlab/akita/v5/mem/control"
	"github.com/sarchlab/akita/v5/messaging"
	"github.com/sarchlab/akita/v5/modeling"
	"github.com/sarchlab/akita/v5/timing"
)

// This file holds Layer-2 control-behavior tests: it asserts the actual
// behavior the universal verbs promise (Drain quiescence, Pause freeze,
// Reset from every state), beyond the protocol-surface checks in
// control_contract_test.go.
var _ = Describe("Ideal Memory Controller control behavior", func() {
	var (
		engine        timing.Engine
		storage       *mem.Storage
		memController *Comp
		topPort       messaging.Port
		ctrlPort      messaging.Port
	)

	build := func() {
		spec := DefaultSpec()
		spec.Width = 4
		spec.Latency = 10
		spec.CacheLineSize = 64
		spec.TopPortBufferSize = 16

		memController = MakeBuilder().
			WithRegistrar(modeling.NewStandaloneRegistrar(engine)).
			WithResources(Resources{Storage: storage}).
			WithSpec(spec).
			Build("MemCtrl")

		topPort = memController.GetPortByName("Top")
		ctrlPort = memController.GetPortByName("Control")
		for _, p := range []messaging.Port{topPort, ctrlPort} {
			(&noopConn{}).PlugIn(p)
		}
	}

	makeRead := func(addr uint64) *mem.ReadReq {
		req := &mem.ReadReq{}
		req.ID = timing.GetIDGenerator().Generate()
		req.Src = messaging.RemotePort("Agent")
		req.Dst = topPort.AsRemote()
		req.Address = addr
		req.AccessByteSize = 4
		req.TrafficBytes = 12
		req.TrafficClass = "mem.ReadReq"
		return req
	}

	makeCtrlReq := func(cmd mem.ControlCommand) *mem.ControlReq {
		req := &mem.ControlReq{Command: cmd}
		req.ID = timing.GetIDGenerator().Generate()
		req.Src = messaging.RemotePort("Ctrl")
		req.Dst = ctrlPort.AsRemote()
		req.TrafficClass = "mem.ControlReq"
		return req
	}

	BeforeEach(func() {
		engine = timing.NewSerialEngine()
		storage = mem.NewStorage(1 * mem.MB)
		build()
	})

	It("drains all in-flight reads before acking Drain", func() {
		const n = 3
		for i := range n {
			topPort.Deliver(makeRead(uint64(i * 64)))
		}
		memController.Tick()
		Expect(memController.State.InflightTransactions).To(HaveLen(n))

		drain := makeCtrlReq(mem.CmdDrain)
		ctrlPort.Deliver(drain)

		completed := 0
		var drainRsp *mem.ControlRsp
		for i := 0; i < 4096 && drainRsp == nil; i++ {
			memController.Tick()
			for {
				out := topPort.RetrieveOutgoing()
				if out == nil {
					break
				}
				if _, ok := out.(*mem.DataReadyRsp); ok {
					completed++
				}
			}
			if out := ctrlPort.RetrieveOutgoing(); out != nil {
				if rsp, ok := out.(*mem.ControlRsp); ok &&
					rsp.Command == mem.CmdDrain {
					drainRsp = rsp
				}
			}
		}

		Expect(drainRsp).ToNot(BeNil())
		Expect(drainRsp.Success).To(BeTrue())
		Expect(drainRsp.RspTo).To(Equal(drain.ID))
		// Every in-flight read finished, and none remain, by the time the
		// async Drain ack is sent.
		Expect(completed).To(Equal(n))
		Expect(memController.State.InflightTransactions).To(BeEmpty())
		Expect(memController.State.ControlState).To(Equal(control.StatePaused))
	})

	It("freezes incoming traffic while paused", func() {
		memController.State.ControlState = control.StatePaused
		topPort.Deliver(makeRead(0))

		for range 5 {
			memController.Tick()
		}

		// The request is neither consumed nor turned into work, and no
		// response is produced, while paused.
		Expect(topPort.PeekIncoming()).ToNot(BeNil())
		Expect(memController.State.InflightTransactions).To(BeEmpty())
		Expect(topPort.RetrieveOutgoing()).To(BeNil())
	})

	DescribeTable("Reset wipes in-flight state from any control state",
		func(startState control.State) {
			topPort.Deliver(makeRead(0))
			memController.Tick()
			Expect(memController.State.InflightTransactions).ToNot(BeEmpty())

			memController.State.ControlState = startState

			reset := makeCtrlReq(mem.CmdReset)
			ctrlPort.Deliver(reset)

			var rsp *mem.ControlRsp
			for i := 0; i < 64 && rsp == nil; i++ {
				memController.Tick()
				if out := ctrlPort.RetrieveOutgoing(); out != nil {
					rsp, _ = out.(*mem.ControlRsp)
				}
			}

			Expect(rsp).ToNot(BeNil())
			Expect(rsp.Command).To(Equal(mem.CmdReset))
			Expect(rsp.Success).To(BeTrue())
			Expect(rsp.RspTo).To(Equal(reset.ID))
			Expect(memController.State.InflightTransactions).To(BeEmpty())
			Expect(memController.State.ControlState).
				To(Equal(control.StateEnabled))
		},
		Entry("from Enabled", control.StateEnabled),
		Entry("from Paused", control.StatePaused),
		Entry("from Draining", control.StateDraining),
	)
})
