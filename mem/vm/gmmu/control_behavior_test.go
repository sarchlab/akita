package gmmu

import (
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/sarchlab/akita/v5/mem"
	"github.com/sarchlab/akita/v5/mem/control"
	"github.com/sarchlab/akita/v5/mem/vm"
	"github.com/sarchlab/akita/v5/messaging"
	"github.com/sarchlab/akita/v5/modeling"
	"github.com/sarchlab/akita/v5/timing"
)

// This file holds Layer-2 control-behavior tests for the GMMU: it asserts the
// actual behavior the universal verbs promise (Drain quiescence, Pause freeze,
// Reset from every state), beyond the protocol-surface checks in
// control_contract_test.go.
//
// All translation requests target locally-MAPPED pages (page.DeviceID equal to
// the GMMU's spec.DeviceID), so each walk self-completes via finalizePageWalk
// and emits a *vm.TranslationRsp on Top without ever sending a remote memory
// request out Bottom. That keeps RemoteMemReqs empty and lets Drain reach
// quiescence purely on the local walk path.
var _ = Describe("GMMU control behavior", func() {
	var (
		engine    timing.Engine
		pageTable vm.PageTable
		comp      *Comp
		topPort   messaging.Port
		ctrlPort  messaging.Port
	)

	const (
		agentPort = messaging.RemotePort("Agent")
		lowModule = messaging.RemotePort("LowModule")
		deviceID  = uint64(0)
	)

	build := func() {
		spec := DefaultSpec()
		spec.DeviceID = deviceID
		// A latency larger than the in-flight count lets every delivered
		// request be parsed into a walk before the first one finalizes, so
		// all N walks can be observed in flight at once.
		spec.Latency = 10
		spec.LowModule = lowModule

		reg := modeling.NewStandaloneRegistrar(engine)
		comp = MakeBuilder().
			WithRegistrar(reg).
			WithResources(Resources{PageTable: pageTable}).
			WithSpec(spec).
			Build("GMMU")

		assignDefaultPorts(reg, comp)

		topPort = comp.GetPortByName("Top")
		ctrlPort = comp.GetPortByName("Control")
		for _, name := range []string{"Top", "Bottom", "Control"} {
			(&noopConn{}).PlugIn(comp.GetPortByName(name))
		}
	}

	// insertLocalPage maps vAddr to a page owned by this GMMU's device, so the
	// walk resolves locally (no remote request out Bottom).
	insertLocalPage := func(vAddr uint64) {
		pageTable.Insert(vm.Page{
			PID:      vm.PID(1),
			VAddr:    vAddr,
			DeviceID: deviceID,
			Valid:    true,
		})
	}

	makeTranslationReq := func(vAddr uint64) vm.TranslationReq {
		req := vm.TranslationReq{}
		req.ID = timing.GetIDGenerator().Generate()
		req.Src = agentPort
		req.Dst = topPort.AsRemote()
		req.PID = 1
		req.VAddr = vAddr
		req.DeviceID = deviceID
		req.TrafficClass = "vm.TranslationReq"
		return req
	}

	makeCtrlReq := func(cmd mem.ControlCommand) mem.ControlReq {
		req := mem.ControlReq{Command: cmd}
		req.ID = timing.GetIDGenerator().Generate()
		req.Src = messaging.RemotePort("Ctrl")
		req.Dst = ctrlPort.AsRemote()
		req.TrafficClass = "mem.ControlReq"
		return req
	}

	BeforeEach(func() {
		engine = timing.NewSerialEngine()
		pageTable = vm.NewPageTable(12)
		build()
	})

	It("drains all in-flight translations before acking Drain", func() {
		const n = 3
		for i := range n {
			vAddr := uint64(i) << 12
			insertLocalPage(vAddr)
			topPort.Deliver(makeTranslationReq(vAddr))
		}

		// Tick until every delivered request has been parsed into an
		// in-flight walk, so the Drain that follows must wait for real work.
		for i := 0; i < 64 &&
			len(comp.State.WalkingTranslations) < n; i++ {
			comp.Tick()
		}
		Expect(comp.State.WalkingTranslations).To(HaveLen(n))

		drain := makeCtrlReq(mem.CmdDrain)
		ctrlPort.Deliver(drain)

		completed := 0
		var drainRsp mem.ControlRsp
		gotDrainRsp := false
		for i := 0; i < 4096 && !gotDrainRsp; i++ {
			comp.Tick()
			for {
				out := topPort.RetrieveOutgoing()
				if out == nil {
					break
				}
				if _, ok := out.(vm.TranslationRsp); ok {
					completed++
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

		Expect(gotDrainRsp).To(BeTrue())
		Expect(drainRsp.Success).To(BeTrue())
		Expect(drainRsp.RspTo).To(Equal(drain.ID))
		// Every in-flight walk finished locally, and none remain, by the time
		// the async Drain ack is sent.
		Expect(completed).To(Equal(n))
		Expect(comp.State.WalkingTranslations).To(BeEmpty())
		Expect(comp.State.RemoteMemReqs).To(HaveLen(0))
		Expect(comp.State.ControlState).To(Equal(control.StatePaused))
	})

	It("freezes incoming translations while paused", func() {
		comp.State.ControlState = control.StatePaused

		insertLocalPage(0x0)
		topPort.Deliver(makeTranslationReq(0x0))

		for range 5 {
			comp.Tick()
		}

		// The request is neither consumed nor turned into a walk, and no
		// translation response is produced, while paused.
		Expect(topPort.PeekIncoming()).ToNot(BeNil())
		Expect(comp.State.WalkingTranslations).To(BeEmpty())
		Expect(topPort.RetrieveOutgoing()).To(BeNil())
	})

	DescribeTable("Reset wipes in-flight state from any control state",
		func(startState control.State) {
			insertLocalPage(0x0)
			topPort.Deliver(makeTranslationReq(0x0))

			// Get a walk in flight before Reset arrives.
			for i := 0; i < 64 &&
				len(comp.State.WalkingTranslations) == 0; i++ {
				comp.Tick()
			}
			Expect(comp.State.WalkingTranslations).ToNot(BeEmpty())

			comp.State.ControlState = startState

			reset := makeCtrlReq(mem.CmdReset)
			ctrlPort.Deliver(reset)

			var rsp mem.ControlRsp
			gotRsp := false
			for i := 0; i < 64 && !gotRsp; i++ {
				comp.Tick()
				if out := ctrlPort.RetrieveOutgoing(); out != nil {
					rsp, gotRsp = out.(mem.ControlRsp)
				}
			}

			Expect(gotRsp).To(BeTrue())
			Expect(rsp.Command).To(Equal(mem.CmdReset))
			Expect(rsp.Success).To(BeTrue())
			Expect(rsp.RspTo).To(Equal(reset.ID))
			Expect(comp.State.WalkingTranslations).To(BeEmpty())
			Expect(comp.State.RemoteMemReqs).To(HaveLen(0))
			// RemoteMemReqs must remain a usable (non-nil) map after Reset:
			// walkMW writes to it directly, so a nil map would panic on the
			// next remote walk.
			Expect(comp.State.RemoteMemReqs).ToNot(BeNil())
			Expect(comp.State.ControlState).
				To(Equal(control.StateEnabled))
		},
		Entry("from Enabled", control.StateEnabled),
		Entry("from Paused", control.StatePaused),
		// The Draining case is covered by the dedicated test below: control
		// commands are serialized with no preemption, so a Reset issued while
		// draining is not serviced until the in-progress Drain acks first.
	)

	It("completes a pending Drain before servicing a queued Reset", func() {
		// The component is draining and already quiescent: completePendingDrain
		// acks the Drain. Control commands are serialized with no preemption,
		// so a Reset queued behind the drain is serviced only after the Drain
		// acks.
		comp.State.ControlState = control.StateDraining
		comp.State.CurrentCmdID = 999
		comp.State.CurrentCmdSrc = messaging.RemotePort("Drainer")
		comp.State.WalkingTranslations = nil
		comp.State.RemoteMemReqs = nil

		reset := makeCtrlReq(mem.CmdReset)
		ctrlPort.Deliver(reset)

		var rsps []mem.ControlRsp
		for range 16 {
			comp.Tick()
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
		Expect(comp.State.ControlState).To(Equal(control.StateEnabled))
	})
})
