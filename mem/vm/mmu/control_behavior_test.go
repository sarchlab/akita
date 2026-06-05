package mmu

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

// This file holds Layer-2 control-behavior tests for the MMU: it asserts the
// actual behavior the universal verbs promise (Drain quiescence, Pause freeze,
// Reset from every state), beyond the protocol-surface checks in
// control_contract_test.go.
//
// The MMU resolves a translation against its in-process page table without a
// downstream round-trip, so a walk for a MAPPED page that does not need
// migration is self-completing: it produces a *vm.TranslationRsp on the Top
// port and removes itself from WalkingTranslations. The tests below rely on
// that by inserting pages whose DeviceID matches the request's DeviceID (so
// pageNeedMigrate stays false) and that are not migrating, keeping the walk
// entirely local.
var _ = Describe("MMU control behavior", func() {
	var (
		engine    timing.Engine
		pageTable vm.PageTable
		comp      *Comp
		topPort   messaging.Port
		ctrlPort  messaging.Port
	)

	build := func() {
		spec := DefaultSpec()
		spec.MigrationServiceProvider =
			messaging.RemotePort("MigrationServiceProvider")

		comp = MakeBuilder().
			WithRegistrar(modeling.NewStandaloneRegistrar(engine)).
			WithResources(Resources{PageTable: pageTable}).
			WithSpec(spec).
			Build("MMU")

		topPort = comp.GetPortByName("Top")
		ctrlPort = comp.GetPortByName("Control")
		for _, name := range []string{"Top", "Migration", "Control"} {
			(&noopConn{}).PlugIn(comp.GetPortByName(name))
		}
	}

	// insertMappedPage adds a page that resolves locally: it is not migrating
	// and its DeviceID matches the translation request's DeviceID (0), so the
	// walk never enters the migration path.
	insertMappedPage := func(vAddr uint64) vm.Page {
		page := vm.Page{
			PID:      1,
			VAddr:    vAddr,
			PAddr:    vAddr,
			PageSize: 4096,
			Valid:    true,
			DeviceID: 0,
		}
		pageTable.Insert(page)
		return page
	}

	makeTranslationReq := func(vAddr uint64) vm.TranslationReq {
		req := vm.TranslationReq{}
		req.ID = timing.GetIDGenerator().Generate()
		req.Src = messaging.RemotePort("Agent")
		req.Dst = topPort.AsRemote()
		req.PID = 1
		req.VAddr = vAddr
		req.DeviceID = 0
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
			vAddr := uint64(0x1000 * (i + 1))
			insertMappedPage(vAddr)
			topPort.Deliver(makeTranslationReq(vAddr))
		}

		// Tick until every request has been parsed into a walk.
		for i := 0; i < 4096 &&
			len(comp.State.WalkingTranslations) != n; i++ {
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
		// Every in-flight walk finished, and none remain, by the time the
		// async Drain ack is sent.
		Expect(completed).To(Equal(n))
		Expect(comp.State.WalkingTranslations).To(BeEmpty())
		Expect(comp.State.ControlState).To(Equal(control.StatePaused))
	})

	It("freezes incoming translations while paused", func() {
		comp.State.ControlState = control.StatePaused

		insertMappedPage(0x1000)
		topPort.Deliver(makeTranslationReq(0x1000))

		for range 5 {
			comp.Tick()
		}

		// The request is neither consumed nor turned into a walk, and no
		// response is produced, while paused.
		Expect(topPort.PeekIncoming()).ToNot(BeNil())
		Expect(comp.State.WalkingTranslations).To(BeEmpty())
		Expect(topPort.RetrieveOutgoing()).To(BeNil())
	})

	DescribeTable("Reset wipes in-flight walk state from any control state",
		func(startState control.State) {
			insertMappedPage(0x1000)
			topPort.Deliver(makeTranslationReq(0x1000))

			// Tick until the request is parsed into a walk in flight.
			for i := 0; i < 4096 &&
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
			Expect(comp.State.ControlState).
				To(Equal(control.StateEnabled))
		},
		Entry("from Enabled", control.StateEnabled),
		Entry("from Paused", control.StatePaused),
		Entry("from Draining", control.StateDraining),
	)
})
