package tlb

import (
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/sarchlab/akita/v5/mem"
	"github.com/sarchlab/akita/v5/mem/memcontrolprotocol"
	"github.com/sarchlab/akita/v5/mem/vm"
	"github.com/sarchlab/akita/v5/mem/vm/vmprotocol"
	"github.com/sarchlab/akita/v5/sim/messaging"
	"github.com/sarchlab/akita/v5/sim/modeling"
	"github.com/sarchlab/akita/v5/sim/timing"
)

var _ = Describe("TLB", func() {

	var (
		engine      timing.Engine
		sim         timing.Simulation
		tlbComp     *Comp
		tlbMW       *tlbMiddleware
		tlbCtrlMW   *ctrlMiddleware
		topPort     messaging.Port
		bottomPort  messaging.Port
		controlPort messaging.Port
		remotePort  = messaging.RemotePort("RemotePort")
	)

	BeforeEach(func() {
		engine = timing.NewSerialEngine()
		sim = modeling.NewStandaloneSimulation(engine)

		spec := Definition.DefaultSpec
		spec.NumSets = 1
		spec.NumWays = 32
		spec.Log2PageSize = 12

		tlbComp = Definition.Builder().
			WithSimulation(sim).
			WithSpec(spec).
			WithResources(Resources{
				TranslationProviderMapper: &mem.SinglePortMapper{
					Port: remotePort,
				},
			}).
			Build("TLB")
		tlbCompPorts := defaultPorts("TLB")
		tlbComp.BindPort("Top", tlbCompPorts.Top)
		tlbComp.BindPort("Bottom", tlbCompPorts.Bottom)
		tlbComp.BindPort("Control", tlbCompPorts.Control)
		plugNoopConn(tlbComp)
		if err := sim.Initialize(); err != nil {
			panic(err)
		}

		topPort = tlbComp.Ports.Top
		bottomPort = tlbComp.Ports.Bottom
		controlPort = tlbComp.Ports.Control

		tlbMW = tlbComp.Middlewares.TLB
		tlbCtrlMW = tlbComp.Middlewares.Ctrl
	})

	It("should do nothing if there is no req in TopPort", func() {
		madeProgress := tlbMW.insertIntoPipeline()

		Expect(madeProgress).To(BeFalse())
	})

	It("should insert req into pipeline when topPort has req", func() {
		req := messaging.Msg{Payload: vmprotocol.TranslationReq{
			PID:      1,
			VAddr:    uint64(0x100),
			DeviceID: 1},
			ID:  sim.NewID(),
			Src: messaging.RemotePort("Agent"),
			Dst: topPort.AsRemote(),

			TrafficClass: "vmprotocol.TranslationReq"}

		topPort.Deliver(req)

		madeProgress := tlbMW.insertIntoPipeline()

		Expect(madeProgress).To(BeTrue())
	})

	Context("hit", func() {
		var (
			req messaging.Msg
		)

		BeforeEach(func() {
			// Set up a page in the TLB state
			page := vm.Page{
				PID:   1,
				VAddr: 0x100,
				PAddr: 0x200,
				Valid: true,
			}
			next := &tlbComp.State
			setUpdate(&next.Sets[0], 1, page)
			setVisit(&next.Sets[0], 1)

			req = messaging.Msg{Payload: vmprotocol.TranslationReq{
				PID:      1,
				VAddr:    uint64(0x100),
				DeviceID: 1},
				ID:  sim.NewID(),
				Src: messaging.RemotePort("Agent"),

				TrafficClass: "vmprotocol.TranslationReq"}

		})

		It("should respond to top", func() {
			madeProgress := tlbMW.lookup(req)

			Expect(madeProgress).To(BeTrue())
			rsp, _ := topPort.RetrieveOutgoing()
			Expect(rsp).To(BeAssignableToTypeOf(messaging.Msg{Payload: vmprotocol.TranslationRsp{}}))
		})
	})

	Context("miss", func() {
		var (
			req messaging.Msg
		)

		BeforeEach(func() {
			// Set up a page with Valid=false to trigger miss
			page := vm.Page{
				PID:   1,
				VAddr: 0x100,
				PAddr: 0x200,
				Valid: false,
			}
			next := &tlbComp.State
			setUpdate(&next.Sets[0], 1, page)
			setVisit(&next.Sets[0], 1)

			req = messaging.Msg{Payload: vmprotocol.TranslationReq{
				PID:      1,
				VAddr:    0x100,
				DeviceID: 1},
				ID:  sim.NewID(),
				Src: messaging.RemotePort("Agent"),

				TrafficClass: "vmprotocol.TranslationReq"}

		})

		It("should fetch from bottom and add entry to MSHR", func() {
			madeProgress := tlbMW.lookup(req)

			Expect(madeProgress).To(BeTrue())
			nextState := &tlbComp.State
			Expect(mshrIsEntryPresent(nextState.MSHREntries, vm.PID(1), uint64(0x100))).
				To(Equal(true))

			sent, _ := bottomPort.RetrieveOutgoing()
			Expect(sent).To(BeAssignableToTypeOf(messaging.Msg{Payload: vmprotocol.TranslationReq{}}))
			sentMsg := sent
			Expect(sentMsg.Payload.(vmprotocol.TranslationReq).VAddr).To(Equal(uint64(0x100)))
			Expect(sentMsg.Payload.(vmprotocol.TranslationReq).PID).To(Equal(vm.PID(1)))
			Expect(sentMsg.Payload.(vmprotocol.TranslationReq).DeviceID).To(Equal(uint64(1)))
			Expect(sentMsg.Dst).To(Equal(remotePort))
		})
	})

	Context("parse bottom", func() {
		var (
			req         messaging.Msg
			fetchBottom messaging.Msg
			page        vm.Page
			rsp         messaging.Msg
		)

		BeforeEach(func() {
			req = messaging.Msg{Payload: vmprotocol.TranslationReq{
				PID:      1,
				VAddr:    0x100,
				DeviceID: 1},
				ID:  sim.NewID(),
				Src: messaging.RemotePort("Agent"),

				TrafficClass: "vmprotocol.TranslationReq"}

			fetchBottom = messaging.Msg{Payload: vmprotocol.TranslationReq{
				PID:      1,
				VAddr:    0x100,
				DeviceID: 1},
				ID: sim.NewID(),

				TrafficClass: "vmprotocol.TranslationReq"}

			page = vm.Page{
				PID:   1,
				VAddr: 0x100,
				PAddr: 0x200,
				Valid: true,
			}
			rsp = messaging.Msg{Payload: vmprotocol.TranslationRsp{
				Page: page,
			},
				ID:           sim.NewID(),
				Src:          messaging.RemotePort("Agent"),
				Dst:          bottomPort.AsRemote(),
				RspTo:        fetchBottom.ID,
				TrafficClass: "vmprotocol.TranslationRsp"}

		})

		It("should do nothing if no return", func() {
			madeProgress := tlbMW.parseBottom()

			Expect(madeProgress).To(BeFalse())
		})

		It("should stall if the TLB is responding to an MSHR entry", func() {
			// Set up responding MSHR entry
			next := &tlbComp.State
			next.HasRespondingMSHR = true
			next.RespondingMSHRData = mshrEntryState{
				PID:      1,
				VAddr:    0x100,
				Requests: []messaging.Msg{req},
			}
			// Also add the MSHR entry
			next.MSHREntries, _ = mshrAdd(next.MSHREntries, 4, 1, 0x100)

			madeProgress := tlbMW.parseBottom()

			Expect(madeProgress).To(BeFalse())
		})

		It("should parse respond from bottom", func() {
			bottomPort.Deliver(rsp)

			// Add MSHR entry
			next := &tlbComp.State
			next.MSHREntries, _ = mshrAdd(next.MSHREntries, 4, 1, 0x100)
			idx, _ := mshrGetEntry(next.MSHREntries, 1, 0x100)
			next.MSHREntries[idx].Requests = append(next.MSHREntries[idx].Requests, req)
			next.MSHREntries[idx].HasReqToBottom = true
			next.MSHREntries[idx].ReqToBottom = fetchBottom

			madeProgress := tlbMW.parseBottom()

			Expect(madeProgress).To(BeTrue())
			nextState := &tlbComp.State
			Expect(nextState.HasRespondingMSHR).To(BeTrue())
			Expect(mshrIsEntryPresent(nextState.MSHREntries, vm.PID(1), uint64(0x100))).
				To(Equal(false))
		})

		It("should respond", func() {
			next := &tlbComp.State
			next.HasRespondingMSHR = true
			next.RespondingMSHRData = mshrEntryState{
				PID:      1,
				VAddr:    0x100,
				Requests: []messaging.Msg{req},
			}

			madeProgress := tlbMW.respondMSHREntry()

			Expect(madeProgress).To(BeTrue())
			nextState := &tlbComp.State
			Expect(nextState.RespondingMSHRData.Requests).To(HaveLen(0))
			Expect(nextState.HasRespondingMSHR).To(BeFalse())

			rsp, _ := topPort.RetrieveOutgoing()
			Expect(rsp).To(BeAssignableToTypeOf(messaging.Msg{Payload: vmprotocol.TranslationRsp{}}))
		})
	})

	Context("flush related handling", func() {

		It("should do nothing if no req", func() {
			madeProgress := tlbCtrlMW.handleIncomingCommands()
			Expect(madeProgress).To(BeFalse())
		})

		It("should invalidate a matching entry when paused", func() {
			next := &tlbComp.State
			next.TLBState = tlbStatePause

			page := vm.Page{
				PID:   1,
				VAddr: 0x1000,
				Valid: true,
			}
			setUpdate(&next.Sets[0], 1, page)
			setVisit(&next.Sets[0], 1)

			invReq := messaging.Msg{Payload: memcontrolprotocol.Req{
				Command:   memcontrolprotocol.CmdInvalidate,
				Addresses: []uint64{0x1000},
				PID:       1,
			},
				ID:           sim.NewID(),
				Src:          messaging.RemotePort("Agent"),
				Dst:          controlPort.AsRemote(),
				TrafficClass: "memcontrolprotocol.Req"}

			controlPort.Deliver(invReq)

			madeProgress := tlbCtrlMW.handleIncomingCommands()
			Expect(madeProgress).To(BeTrue())

			_, gotPage, found := setLookup(&next.Sets[0], 1, 0x1000)
			Expect(found && gotPage.Valid).To(BeFalse())

			rspMsg, _ := controlPort.RetrieveOutgoing()
			Expect(rspMsg).To(BeAssignableToTypeOf(messaging.Msg{Payload: memcontrolprotocol.Rsp{}}))
			rsp := rspMsg
			Expect(rsp.Payload.(memcontrolprotocol.Rsp).Command).To(Equal(memcontrolprotocol.CmdInvalidate))
			Expect(rsp.Payload.(memcontrolprotocol.Rsp).Success).To(BeTrue())
		})

		It("should reject Invalidate while enabled", func() {
			next := &tlbComp.State
			next.TLBState = tlbStateEnable

			invReq := messaging.Msg{Payload: memcontrolprotocol.Req{Command: memcontrolprotocol.CmdInvalidate},
				ID:           sim.NewID(),
				Src:          messaging.RemotePort("Agent"),
				Dst:          controlPort.AsRemote(),
				TrafficClass: "memcontrolprotocol.Req"}

			controlPort.Deliver(invReq)
			Expect(tlbCtrlMW.handleIncomingCommands()).To(BeTrue())

			rspMsg, _ := controlPort.RetrieveOutgoing()
			rsp := rspMsg
			Expect(rsp.Payload.(memcontrolprotocol.Rsp).Success).To(BeFalse())
			Expect(rsp.Payload.(memcontrolprotocol.Rsp).Error).To(Equal(memcontrolprotocol.ErrMustBePausedOrDrained))
		})

		It("invalidates only entries matching the PID filter", func() {
			next := &tlbComp.State
			next.TLBState = tlbStatePause

			pageA := vm.Page{PID: 1, VAddr: 0x1000, Valid: true}
			pageB := vm.Page{PID: 2, VAddr: 0x2000, Valid: true}
			setUpdate(&next.Sets[0], 0, pageA)
			setVisit(&next.Sets[0], 0)
			setUpdate(&next.Sets[0], 1, pageB)
			setVisit(&next.Sets[0], 1)

			invReq := messaging.Msg{Payload: memcontrolprotocol.Req{Command: memcontrolprotocol.CmdInvalidate, PID: 1},
				ID:           sim.NewID(),
				Src:          messaging.RemotePort("Agent"),
				Dst:          controlPort.AsRemote(),
				TrafficClass: "memcontrolprotocol.Req"}

			controlPort.Deliver(invReq)

			Expect(tlbCtrlMW.handleIncomingCommands()).To(BeTrue())

			// Only the PID-1 entry is dropped; the PID-2 entry survives.
			Expect(next.Sets[0].Blocks[0].Page.Valid).To(BeFalse())
			Expect(next.Sets[0].Blocks[1].Page.Valid).To(BeTrue())

			rspValue, _ := controlPort.RetrieveOutgoing()

			rsp := rspValue
			Expect(rsp.Payload.(memcontrolprotocol.Rsp).Command).To(Equal(memcontrolprotocol.CmdInvalidate))
			Expect(rsp.Payload.(memcontrolprotocol.Rsp).Success).To(BeTrue())
		})

		It("clears cached entries on Reset", func() {
			next := &tlbComp.State
			page := vm.Page{PID: 1, VAddr: 0x1000, Valid: true}
			setUpdate(&next.Sets[0], 0, page)
			setVisit(&next.Sets[0], 0)

			resetReq := messaging.Msg{Payload: memcontrolprotocol.Req{Command: memcontrolprotocol.CmdReset},
				ID:           sim.NewID(),
				Src:          messaging.RemotePort("Agent"),
				Dst:          controlPort.AsRemote(),
				TrafficClass: "memcontrolprotocol.Req"}

			controlPort.Deliver(resetReq)

			Expect(tlbCtrlMW.handleIncomingCommands()).To(BeTrue())

			// Reset returns the TLB to its freshly-built empty state.
			_, gotPage, found := setLookup(&next.Sets[0], 1, 0x1000)
			Expect(found && gotPage.Valid).To(BeFalse())

			rspValue, _ := controlPort.RetrieveOutgoing()

			rsp := rspValue
			Expect(rsp.Payload.(memcontrolprotocol.Rsp).Command).To(Equal(memcontrolprotocol.CmdReset))
			Expect(rsp.Payload.(memcontrolprotocol.Rsp).Success).To(BeTrue())
		})

		It("should handle restart request", func() {
			restartReq := messaging.Msg{Payload: memcontrolprotocol.Req{Command: memcontrolprotocol.CmdReset},
				ID:           sim.NewID(),
				Src:          messaging.RemotePort("Agent"),
				Dst:          controlPort.AsRemote(),
				TrafficClass: "memcontrolprotocol.Req"}

			controlPort.Deliver(restartReq)

			madeProgress := tlbCtrlMW.handleIncomingCommands()

			Expect(madeProgress).To(BeTrue())

			rsp, _ := controlPort.RetrieveOutgoing()
			Expect(rsp).To(BeAssignableToTypeOf(messaging.Msg{Payload: memcontrolprotocol.Rsp{}}))
		})
	})

	Context("other control signals", func() {
		It("should handle pause ctrl msg", func() {
			pauseMsg := messaging.Msg{Payload: memcontrolprotocol.Req{
				Command: memcontrolprotocol.CmdPause,
			},
				ID:           sim.NewID(),
				Src:          messaging.RemotePort("Agent"),
				Dst:          controlPort.AsRemote(),
				TrafficBytes: 4,
				TrafficClass: "memcontrolprotocol.Req"}

			controlPort.Deliver(pauseMsg)

			madeProgress := tlbCtrlMW.handleIncomingCommands()

			Expect(madeProgress).To(BeTrue())
			nextState := &tlbComp.State
			Expect(nextState.TLBState).To(Equal(tlbStatePause))
		})

		It("should handle enable ctrl msg after pause", func() {
			pause := messaging.Msg{Payload: memcontrolprotocol.Req{
				Command: memcontrolprotocol.CmdPause,
			},
				ID:           sim.NewID(),
				Src:          messaging.RemotePort("Agent"),
				Dst:          controlPort.AsRemote(),
				TrafficBytes: 4,
				TrafficClass: "memcontrolprotocol.Req"}

			controlPort.Deliver(pause)

			madeProgress := tlbCtrlMW.handleIncomingCommands()

			Expect(madeProgress).To(BeTrue())
			nextState := &tlbComp.State
			Expect(nextState.TLBState).To(Equal(tlbStatePause))

			// Drain the Pause ack so the next ack has room in the
			// single-slot control port outgoing buffer.
			value0, present0 := controlPort.RetrieveOutgoing()
			Expect(present0).To(BeTrue())
			Expect(value0).
				To(BeAssignableToTypeOf(messaging.Msg{Payload: memcontrolprotocol.Rsp{}}))

			enable := messaging.Msg{Payload: memcontrolprotocol.Req{
				Command: memcontrolprotocol.CmdEnable,
			},
				ID:           sim.NewID(),
				Src:          messaging.RemotePort("Agent"),
				Dst:          controlPort.AsRemote(),
				TrafficBytes: 4,
				TrafficClass: "memcontrolprotocol.Req"}

			controlPort.Deliver(enable)

			madeProgress = tlbCtrlMW.handleIncomingCommands()
			Expect(madeProgress).To(BeTrue())
			nextState = &tlbComp.State
			Expect(nextState.TLBState).To(Equal(tlbStateEnable))
		})

		It("should handle drain ctrl msg", func() {
			drainMsg := messaging.Msg{Payload: memcontrolprotocol.Req{
				Command: memcontrolprotocol.CmdDrain,
			},
				ID:           sim.NewID(),
				Src:          messaging.RemotePort("Agent"),
				Dst:          controlPort.AsRemote(),
				TrafficBytes: 4,
				TrafficClass: "memcontrolprotocol.Req"}

			controlPort.Deliver(drainMsg)

			madeProgress := tlbCtrlMW.handleIncomingCommands()

			Expect(madeProgress).To(BeTrue())
			nextState := &tlbComp.State
			Expect(nextState.TLBState).To(Equal(tlbStateDrain))

			madeProgress = tlbMW.handleDrain()
			Expect(madeProgress).To(BeFalse())
			nextState = &tlbComp.State
			Expect(nextState.TLBState).To(Equal(tlbStatePause))
		})
	})
})

var _ = Describe("TLB Integration", func() {
	var (
		engine     timing.Engine
		sim        timing.Simulation
		tlbComp    *Comp
		lowModule  *idealEndpoint
		agent      *idealEndpoint
		connection messaging.Connection
		page       vm.Page
	)

	BeforeEach(func() {
		engine = timing.NewSerialEngine()
		sim = modeling.NewStandaloneSimulation(engine)

		connection = makeDirectConnection(sim)

		lowModule = newIdealEndpoint("LowModule")
		agent = newIdealEndpoint("Agent")

		tlbComp = Definition.Builder().
			WithSimulation(sim).
			WithSpec(Definition.DefaultSpec).
			WithResources(Resources{
				TranslationProviderMapper: &mem.SinglePortMapper{
					Port: lowModule.port.AsRemote(),
				},
			}).
			Build("TLB")
		tlbCompPorts := defaultPorts("TLB")
		tlbComp.BindPort("Top", tlbCompPorts.Top)
		tlbComp.BindPort("Bottom", tlbCompPorts.Bottom)
		tlbComp.BindPort("Control", tlbCompPorts.Control)

		connection.BindPort(agent.port)
		connection.BindPort(lowModule.port)
		connection.BindPort(tlbComp.Ports.Top)
		connection.BindPort(tlbComp.Ports.Bottom)
		connection.BindPort(tlbComp.Ports.Control)
		if err := sim.Initialize(); err != nil {
			panic(err)
		}

		page = vm.Page{
			PID:   1,
			VAddr: 0x1000,
			PAddr: 0x2000,
			Valid: true,
		}

		// lowModule answers every translation request with the page.
		lowModule.onDeliver = func(msg messaging.Msg) {
			translationReq := msg
			rsp := messaging.Msg{Payload: vmprotocol.TranslationRsp{Page: page},
				ID:           sim.NewID(),
				Src:          lowModule.port.AsRemote(),
				Dst:          translationReq.Src,
				RspTo:        translationReq.ID,
				TrafficClass: "vmprotocol.TranslationRsp"}

			lowModule.port.Send(rsp)
		}
	})

	It("should do tlb miss", func() {
		req := messaging.Msg{Payload: vmprotocol.TranslationReq{
			PID:      1,
			VAddr:    0x1000,
			DeviceID: 1},
			ID:  sim.NewID(),
			Src: agent.port.AsRemote(),
			Dst: tlbComp.Ports.Top.AsRemote(),

			TrafficClass: "vmprotocol.TranslationReq"}

		agent.port.Send(req)

		Expect(engine.Run()).To(Succeed())

		rsp := agent.lastDelivered
		Expect(rsp.Payload.(vmprotocol.TranslationRsp).Page).To(Equal(page))
	})

	It("should have faster hit than miss", func() {
		time1 := engine.CurrentTime()
		req := messaging.Msg{Payload: vmprotocol.TranslationReq{
			PID:      1,
			VAddr:    0x1000,
			DeviceID: 1},
			ID:  sim.NewID(),
			Src: agent.port.AsRemote(),
			Dst: tlbComp.Ports.Top.AsRemote(),

			TrafficClass: "vmprotocol.TranslationReq"}

		agent.port.Send(req)

		Expect(engine.Run()).To(Succeed())

		rsp := agent.lastDelivered
		Expect(rsp.Payload.(vmprotocol.TranslationRsp).Page).To(Equal(page))

		time2 := engine.CurrentTime()

		req2 := messaging.Msg{Payload: vmprotocol.TranslationReq{
			PID:      1,
			VAddr:    0x1000,
			DeviceID: 1},
			ID:  sim.NewID(),
			Src: agent.port.AsRemote(),
			Dst: tlbComp.Ports.Top.AsRemote(),

			TrafficClass: "vmprotocol.TranslationReq"}

		agent.port.Send(req2)

		Expect(engine.Run()).To(Succeed())

		rsp = agent.lastDelivered
		Expect(rsp.Payload.(vmprotocol.TranslationRsp).Page).To(Equal(page))

		time3 := engine.CurrentTime()

		Expect(time3 - time2).To(BeNumerically("<", time2-time1))
	})
})
