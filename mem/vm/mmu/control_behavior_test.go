package mmu

import (
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/sarchlab/akita/v5/mem/control"
	"github.com/sarchlab/akita/v5/mem/vm"
	"github.com/sarchlab/akita/v5/mem/vm/vmprotocol"
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
// downstream round-trip, so a walk for a MAPPED page is self-completing: it
// produces a vmprotocol.TranslationRsp on the Top port and removes itself from
// WalkingTranslations. The tests below rely on that by inserting mapped pages,
// keeping the walk entirely local.
var _ = Describe("MMU control behavior", func() {
	var (
		engine    timing.Engine
		pageTable vm.PageTable
		comp      *Comp
		topPort   messaging.Port
		ctrlPort  messaging.Port
	)

	build := func() {
		reg := modeling.NewStandaloneRegistrar(engine)

		comp = MakeBuilder().
			WithRegistrar(reg).
			WithResources(Resources{PageTable: pageTable}).
			WithSpec(DefaultSpec()).
			Build("MMU")

		topPort = assignPort(reg, comp, "Top", 16)
		ctrlPort = assignPort(reg, comp, "Control", 4)
		for _, name := range []string{"Top", "Control"} {
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

	makeTranslationReq := func(vAddr uint64) vmprotocol.TranslationReq {
		req := vmprotocol.TranslationReq{}
		req.ID = timing.GetIDGenerator().Generate()
		req.Src = messaging.RemotePort("Agent")
		req.Dst = topPort.AsRemote()
		req.PID = 1
		req.VAddr = vAddr
		req.DeviceID = 0
		req.TrafficClass = "vmprotocol.TranslationReq"
		return req
	}

	makeCtrlReq := func(cmd control.Command) control.Req {
		req := control.Req{Command: cmd}
		req.ID = timing.GetIDGenerator().Generate()
		req.Src = messaging.RemotePort("Ctrl")
		req.Dst = ctrlPort.AsRemote()
		req.TrafficClass = "control.Req"
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

		drain := makeCtrlReq(control.CmdDrain)
		ctrlPort.Deliver(drain)

		completed := 0
		var drainRsp control.Rsp
		gotDrainRsp := false
		for i := 0; i < 4096 && !gotDrainRsp; i++ {
			comp.Tick()
			for {
				out := topPort.RetrieveOutgoing()
				if out == nil {
					break
				}
				if _, ok := out.(vmprotocol.TranslationRsp); ok {
					completed++
				}
			}
			if out := ctrlPort.RetrieveOutgoing(); out != nil {
				if rsp, ok := out.(control.Rsp); ok &&
					rsp.Command == control.CmdDrain {
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

			reset := makeCtrlReq(control.CmdReset)
			ctrlPort.Deliver(reset)

			var rsp control.Rsp
			gotRsp := false
			for i := 0; i < 64 && !gotRsp; i++ {
				comp.Tick()
				if out := ctrlPort.RetrieveOutgoing(); out != nil {
					rsp, gotRsp = out.(control.Rsp)
				}
			}

			Expect(gotRsp).To(BeTrue())
			Expect(rsp.Command).To(Equal(control.CmdReset))
			Expect(rsp.Success).To(BeTrue())
			Expect(rsp.RspTo).To(Equal(reset.ID))
			Expect(comp.State.WalkingTranslations).To(BeEmpty())
			Expect(comp.State.ControlState).
				To(Equal(control.StateEnabled))
		},
		Entry("from Enabled", control.StateEnabled),
		Entry("from Paused", control.StatePaused),
		// The Draining case is covered by the dedicated "completes a pending
		// Drain before servicing a queued Reset" test below: under strict
		// serialization a Reset queued during a drain waits for the drain to
		// ack, so this shared closure (which forces an in-flight walk then sets
		// the state) no longer models the draining path.
	)

	It("completes a pending Drain before servicing a queued Reset", func() {
		// The component is draining and already quiescent (no in-flight
		// walks): completePendingDrain acks the Drain. Control commands are
		// serialized with no preemption, so a Reset queued behind the drain is
		// serviced only after the Drain acks.
		comp.State.ControlState = control.StateDraining
		comp.State.CurrentCmdID = 999
		comp.State.CurrentCmdSrc = messaging.RemotePort("Drainer")
		comp.State.WalkingTranslations = nil

		reset := makeCtrlReq(control.CmdReset)
		ctrlPort.Deliver(reset)

		var rsps []control.Rsp
		for range 16 {
			comp.Tick()
			for {
				out := ctrlPort.RetrieveOutgoing()
				if out == nil {
					break
				}
				if r, ok := out.(control.Rsp); ok {
					rsps = append(rsps, r)
				}
			}
		}

		// Two acks, in order: the Drain completion (RspTo 999) then the Reset.
		Expect(rsps).To(HaveLen(2))
		Expect(rsps[0].Command).To(Equal(control.CmdDrain))
		Expect(rsps[0].RspTo).To(Equal(uint64(999)))
		Expect(rsps[1].Command).To(Equal(control.CmdReset))
		Expect(rsps[1].RspTo).To(Equal(reset.ID))
		Expect(comp.State.ControlState).To(Equal(control.StateEnabled))
	})
})
