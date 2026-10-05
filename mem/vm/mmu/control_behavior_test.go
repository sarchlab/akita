package mmu

import (
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/sarchlab/akita/v5/mem/memcontrolprotocol"
	"github.com/sarchlab/akita/v5/mem/vm"
	"github.com/sarchlab/akita/v5/mem/vm/vmprotocol"
	"github.com/sarchlab/akita/v5/simulation/messaging"
	"github.com/sarchlab/akita/v5/simulation/modeling"
	"github.com/sarchlab/akita/v5/simulation/modeling/modelingtest"
	"github.com/sarchlab/akita/v5/simulation/timing"
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
		sim       timing.Simulation
		pageTable vm.PageTable
		comp      *Comp
		topPort   messaging.Port
		ctrlPort  messaging.Port
	)

	build := func() {

		comp = Definition.Builder().
			WithSimulation(sim).
			WithResources(Resources{PageTable: pageTable}).
			WithSpec(Definition.DefaultSpec).
			WithPorts(makePorts("MMU", 16)).
			Build("MMU")

		topPort = comp.Ports.Top
		ctrlPort = comp.Ports.Control
		for _, p := range []messaging.Port{topPort, ctrlPort} {
			(&noopConn{}).PlugIn(p)
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

	makeTranslationReq := func(vAddr uint64) messaging.Msg {
		req := messaging.Msg{Payload: vmprotocol.TranslationReq{
			PID:      1,
			VAddr:    vAddr,
			DeviceID: 0},
			ID:  sim.NewID(),
			Src: messaging.RemotePort("Agent"),
			Dst: topPort.AsRemote(),

			TrafficClass: "vmprotocol.TranslationReq"}

		return req
	}

	makeCtrlReq := func(cmd memcontrolprotocol.Command) messaging.Msg {
		req := messaging.Msg{Payload: memcontrolprotocol.Req{Command: cmd},
			ID:           sim.NewID(),
			Src:          messaging.RemotePort("Ctrl"),
			Dst:          ctrlPort.AsRemote(),
			TrafficClass: "memcontrolprotocol.Req"}

		return req
	}

	BeforeEach(func() {
		engine = timing.NewSerialEngine()
		sim = modeling.NewStandaloneSimulation(engine)
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
			modelingtest.Tick(comp)
		}
		Expect(comp.State.WalkingTranslations).To(HaveLen(n))

		drain := makeCtrlReq(memcontrolprotocol.CmdDrain)
		ctrlPort.Deliver(drain)

		completed := 0
		var drainRsp messaging.Msg
		gotDrainRsp := false
		for i := 0; i < 4096 && !gotDrainRsp; i++ {
			modelingtest.Tick(comp)
			for {
				out, ok := topPort.RetrieveOutgoing()
				if !ok {
					break
				}
				if _, ok := out.Payload.(vmprotocol.TranslationRsp); ok {
					completed++
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

		Expect(gotDrainRsp).To(BeTrue())
		Expect(drainRsp.Payload.(memcontrolprotocol.Rsp).Success).To(BeTrue())
		Expect(drainRsp.RspTo).To(Equal(drain.ID))
		// Every in-flight walk finished, and none remain, by the time the
		// async Drain ack is sent.
		Expect(completed).To(Equal(n))
		Expect(comp.State.WalkingTranslations).To(BeEmpty())
		Expect(comp.State.ControlState).To(Equal(memcontrolprotocol.StatePaused))
	})

	It("freezes incoming translations while paused", func() {
		comp.State.ControlState = memcontrolprotocol.StatePaused

		insertMappedPage(0x1000)
		topPort.Deliver(makeTranslationReq(0x1000))

		for range 5 {
			modelingtest.Tick(comp)
		}

		// The request is neither consumed nor turned into a walk, and no
		// response is produced, while paused.
		_, present0 := topPort.PeekIncoming()
		Expect(present0).To(BeTrue())
		Expect(comp.State.WalkingTranslations).To(BeEmpty())
		_, present1 := topPort.RetrieveOutgoing()
		Expect(present1).To(BeFalse())
	})

	DescribeTable("Reset wipes in-flight walk state from any control state",
		func(startState memcontrolprotocol.State) {
			insertMappedPage(0x1000)
			topPort.Deliver(makeTranslationReq(0x1000))

			// Tick until the request is parsed into a walk in flight.
			for i := 0; i < 4096 &&
				len(comp.State.WalkingTranslations) == 0; i++ {
				modelingtest.Tick(comp)
			}
			Expect(comp.State.WalkingTranslations).ToNot(BeEmpty())

			comp.State.ControlState = startState

			reset := makeCtrlReq(memcontrolprotocol.CmdReset)
			ctrlPort.Deliver(reset)

			var rsp messaging.Msg
			gotRsp := false
			for i := 0; i < 64 && !gotRsp; i++ {
				modelingtest.Tick(comp)
				if out, ok := ctrlPort.RetrieveOutgoing(); ok {
					_, gotRsp = out.Payload.(memcontrolprotocol.Rsp)
					rsp = out
				}
			}

			Expect(gotRsp).To(BeTrue())
			Expect(rsp.Payload.(memcontrolprotocol.Rsp).Command).To(Equal(memcontrolprotocol.CmdReset))
			Expect(rsp.Payload.(memcontrolprotocol.Rsp).Success).To(BeTrue())
			Expect(rsp.RspTo).To(Equal(reset.ID))
			Expect(comp.State.WalkingTranslations).To(BeEmpty())
			Expect(comp.State.ControlState).
				To(Equal(memcontrolprotocol.StateEnabled))
		},
		Entry("from Enabled", memcontrolprotocol.StateEnabled),
		Entry("from Paused", memcontrolprotocol.StatePaused),
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
		comp.State.ControlState = memcontrolprotocol.StateDraining
		comp.State.CurrentCmdID = 999
		comp.State.CurrentCmdSrc = messaging.RemotePort("Drainer")
		comp.State.WalkingTranslations = nil

		reset := makeCtrlReq(memcontrolprotocol.CmdReset)
		ctrlPort.Deliver(reset)

		var rsps []messaging.Msg
		for range 16 {
			modelingtest.Tick(comp)
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

		// Two acks, in order: the Drain completion (RspTo 999) then the Reset.
		Expect(rsps).To(HaveLen(2))
		Expect(rsps[0].Payload.(memcontrolprotocol.Rsp).Command).To(Equal(memcontrolprotocol.CmdDrain))
		Expect(rsps[0].RspTo).To(Equal(uint64(999)))
		Expect(rsps[1].Payload.(memcontrolprotocol.Rsp).Command).To(Equal(memcontrolprotocol.CmdReset))
		Expect(rsps[1].RspTo).To(Equal(reset.ID))
		Expect(comp.State.ControlState).To(Equal(memcontrolprotocol.StateEnabled))
	})
})
