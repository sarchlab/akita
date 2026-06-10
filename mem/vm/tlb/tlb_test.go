package tlb

import (
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/sarchlab/akita/v5/mem"
	"github.com/sarchlab/akita/v5/mem/memcontrolprotocol"
	"github.com/sarchlab/akita/v5/mem/vm"
	"github.com/sarchlab/akita/v5/mem/vm/vmprotocol"
	"github.com/sarchlab/akita/v5/messaging"
	"github.com/sarchlab/akita/v5/modeling"
	"github.com/sarchlab/akita/v5/timing"
)

var _ = Describe("TLB", func() {

	var (
		engine      timing.Engine
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

		spec := DefaultSpec()
		spec.NumSets = 1
		spec.NumWays = 32
		spec.Log2PageSize = 12

		reg := modeling.NewStandaloneRegistrar(engine)
		tlbComp = MakeBuilder().
			WithRegistrar(reg).
			WithSpec(spec).
			WithResources(Resources{
				TranslationProviderMapper: &mem.SinglePortMapper{
					Port: remotePort,
				},
			}).
			Build("TLB")

		assignDefaultPorts(reg, tlbComp)
		plugNoopConn(tlbComp)

		topPort = tlbComp.GetPortByName("Top")
		bottomPort = tlbComp.GetPortByName("Bottom")
		controlPort = tlbComp.GetPortByName("Control")

		tlbMW = tlbComp.Middlewares()[1].(*tlbMiddleware)
		tlbCtrlMW = tlbComp.Middlewares()[0].(*ctrlMiddleware)
	})

	It("should do nothing if there is no req in TopPort", func() {
		madeProgress := tlbMW.insertIntoPipeline()

		Expect(madeProgress).To(BeFalse())
	})

	It("should insert req into pipeline when topPort has req", func() {
		req := vmprotocol.TranslationReq{}
		req.ID = timing.GetIDGenerator().Generate()
		req.Src = messaging.RemotePort("Agent")
		req.Dst = topPort.AsRemote()
		req.PID = 1
		req.VAddr = uint64(0x100)
		req.DeviceID = 1
		req.TrafficClass = "vmprotocol.TranslationReq"
		topPort.Deliver(req)

		madeProgress := tlbMW.insertIntoPipeline()

		Expect(madeProgress).To(BeTrue())
	})

	Context("hit", func() {
		var (
			req vmprotocol.TranslationReq
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

			req = vmprotocol.TranslationReq{}
			req.ID = timing.GetIDGenerator().Generate()
			req.Src = messaging.RemotePort("Agent")
			req.PID = 1
			req.VAddr = uint64(0x100)
			req.DeviceID = 1
			req.TrafficClass = "vmprotocol.TranslationReq"
		})

		It("should respond to top", func() {
			madeProgress := tlbMW.lookup(req)

			Expect(madeProgress).To(BeTrue())
			rsp := topPort.RetrieveOutgoing()
			Expect(rsp).To(BeAssignableToTypeOf(vmprotocol.TranslationRsp{}))
		})
	})

	Context("miss", func() {
		var (
			req vmprotocol.TranslationReq
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

			req = vmprotocol.TranslationReq{}
			req.ID = timing.GetIDGenerator().Generate()
			req.Src = messaging.RemotePort("Agent")
			req.PID = 1
			req.VAddr = 0x100
			req.DeviceID = 1
			req.TrafficClass = "vmprotocol.TranslationReq"
		})

		It("should fetch from bottom and add entry to MSHR", func() {
			madeProgress := tlbMW.lookup(req)

			Expect(madeProgress).To(BeTrue())
			nextState := &tlbComp.State
			Expect(mshrIsEntryPresent(nextState.MSHREntries, vm.PID(1), uint64(0x100))).
				To(Equal(true))

			sent := bottomPort.RetrieveOutgoing()
			Expect(sent).To(BeAssignableToTypeOf(vmprotocol.TranslationReq{}))
			sentMsg := sent.(vmprotocol.TranslationReq)
			Expect(sentMsg.VAddr).To(Equal(uint64(0x100)))
			Expect(sentMsg.PID).To(Equal(vm.PID(1)))
			Expect(sentMsg.DeviceID).To(Equal(uint64(1)))
			Expect(sentMsg.Dst).To(Equal(remotePort))
		})
	})

	Context("parse bottom", func() {
		var (
			req         vmprotocol.TranslationReq
			fetchBottom vmprotocol.TranslationReq
			page        vm.Page
			rsp         vmprotocol.TranslationRsp
		)

		BeforeEach(func() {
			req = vmprotocol.TranslationReq{}
			req.ID = timing.GetIDGenerator().Generate()
			req.Src = messaging.RemotePort("Agent")
			req.PID = 1
			req.VAddr = 0x100
			req.DeviceID = 1
			req.TrafficClass = "vmprotocol.TranslationReq"
			fetchBottom = vmprotocol.TranslationReq{}
			fetchBottom.ID = timing.GetIDGenerator().Generate()
			fetchBottom.PID = 1
			fetchBottom.VAddr = 0x100
			fetchBottom.DeviceID = 1
			fetchBottom.TrafficClass = "vmprotocol.TranslationReq"
			page = vm.Page{
				PID:   1,
				VAddr: 0x100,
				PAddr: 0x200,
				Valid: true,
			}
			rsp = vmprotocol.TranslationRsp{
				Page: page,
			}
			rsp.ID = timing.GetIDGenerator().Generate()
			rsp.Src = messaging.RemotePort("Agent")
			rsp.Dst = bottomPort.AsRemote()
			rsp.RspTo = fetchBottom.ID
			rsp.TrafficClass = "vmprotocol.TranslationRsp"
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
				Requests: []vmprotocol.TranslationReq{req},
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
				Requests: []vmprotocol.TranslationReq{req},
			}

			madeProgress := tlbMW.respondMSHREntry()

			Expect(madeProgress).To(BeTrue())
			nextState := &tlbComp.State
			Expect(nextState.RespondingMSHRData.Requests).To(HaveLen(0))
			Expect(nextState.HasRespondingMSHR).To(BeFalse())

			rsp := topPort.RetrieveOutgoing()
			Expect(rsp).To(BeAssignableToTypeOf(vmprotocol.TranslationRsp{}))
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

			invReq := memcontrolprotocol.Req{
				Command:   memcontrolprotocol.CmdInvalidate,
				Addresses: []uint64{0x1000},
				PID:       1,
			}
			invReq.ID = timing.GetIDGenerator().Generate()
			invReq.Src = messaging.RemotePort("Agent")
			invReq.Dst = controlPort.AsRemote()
			invReq.TrafficClass = "memcontrolprotocol.Req"

			controlPort.Deliver(invReq)

			madeProgress := tlbCtrlMW.handleIncomingCommands()
			Expect(madeProgress).To(BeTrue())

			_, gotPage, found := setLookup(&next.Sets[0], 1, 0x1000)
			Expect(found && gotPage.Valid).To(BeFalse())

			rspMsg := controlPort.RetrieveOutgoing()
			Expect(rspMsg).To(BeAssignableToTypeOf(memcontrolprotocol.Rsp{}))
			rsp := rspMsg.(memcontrolprotocol.Rsp)
			Expect(rsp.Command).To(Equal(memcontrolprotocol.CmdInvalidate))
			Expect(rsp.Success).To(BeTrue())
		})

		It("should reject Invalidate while enabled", func() {
			next := &tlbComp.State
			next.TLBState = tlbStateEnable

			invReq := memcontrolprotocol.Req{Command: memcontrolprotocol.CmdInvalidate}
			invReq.ID = timing.GetIDGenerator().Generate()
			invReq.Src = messaging.RemotePort("Agent")
			invReq.Dst = controlPort.AsRemote()
			invReq.TrafficClass = "memcontrolprotocol.Req"

			controlPort.Deliver(invReq)
			Expect(tlbCtrlMW.handleIncomingCommands()).To(BeTrue())

			rspMsg := controlPort.RetrieveOutgoing()
			rsp := rspMsg.(memcontrolprotocol.Rsp)
			Expect(rsp.Success).To(BeFalse())
			Expect(rsp.Error).To(Equal(memcontrolprotocol.ErrMustBePausedOrDrained))
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

			invReq := memcontrolprotocol.Req{Command: memcontrolprotocol.CmdInvalidate, PID: 1}
			invReq.ID = timing.GetIDGenerator().Generate()
			invReq.Src = messaging.RemotePort("Agent")
			invReq.Dst = controlPort.AsRemote()
			invReq.TrafficClass = "memcontrolprotocol.Req"
			controlPort.Deliver(invReq)

			Expect(tlbCtrlMW.handleIncomingCommands()).To(BeTrue())

			// Only the PID-1 entry is dropped; the PID-2 entry survives.
			Expect(next.Sets[0].Blocks[0].Page.Valid).To(BeFalse())
			Expect(next.Sets[0].Blocks[1].Page.Valid).To(BeTrue())

			rsp := controlPort.RetrieveOutgoing().(memcontrolprotocol.Rsp)
			Expect(rsp.Command).To(Equal(memcontrolprotocol.CmdInvalidate))
			Expect(rsp.Success).To(BeTrue())
		})

		It("clears cached entries on Reset", func() {
			next := &tlbComp.State
			page := vm.Page{PID: 1, VAddr: 0x1000, Valid: true}
			setUpdate(&next.Sets[0], 0, page)
			setVisit(&next.Sets[0], 0)

			resetReq := memcontrolprotocol.Req{Command: memcontrolprotocol.CmdReset}
			resetReq.ID = timing.GetIDGenerator().Generate()
			resetReq.Src = messaging.RemotePort("Agent")
			resetReq.Dst = controlPort.AsRemote()
			resetReq.TrafficClass = "memcontrolprotocol.Req"
			controlPort.Deliver(resetReq)

			Expect(tlbCtrlMW.handleIncomingCommands()).To(BeTrue())

			// Reset returns the TLB to its freshly-built empty state.
			_, gotPage, found := setLookup(&next.Sets[0], 1, 0x1000)
			Expect(found && gotPage.Valid).To(BeFalse())

			rsp := controlPort.RetrieveOutgoing().(memcontrolprotocol.Rsp)
			Expect(rsp.Command).To(Equal(memcontrolprotocol.CmdReset))
			Expect(rsp.Success).To(BeTrue())
		})

		It("should handle restart request", func() {
			restartReq := memcontrolprotocol.Req{Command: memcontrolprotocol.CmdReset}
			restartReq.ID = timing.GetIDGenerator().Generate()
			restartReq.Src = messaging.RemotePort("Agent")
			restartReq.Dst = controlPort.AsRemote()
			restartReq.TrafficClass = "memcontrolprotocol.Req"
			controlPort.Deliver(restartReq)

			madeProgress := tlbCtrlMW.handleIncomingCommands()

			Expect(madeProgress).To(BeTrue())

			rsp := controlPort.RetrieveOutgoing()
			Expect(rsp).To(BeAssignableToTypeOf(memcontrolprotocol.Rsp{}))
		})
	})

	Context("other control signals", func() {
		It("should handle pause ctrl msg", func() {
			pauseMsg := memcontrolprotocol.Req{
				Command: memcontrolprotocol.CmdPause,
			}
			pauseMsg.ID = timing.GetIDGenerator().Generate()
			pauseMsg.Src = messaging.RemotePort("Agent")
			pauseMsg.Dst = controlPort.AsRemote()
			pauseMsg.TrafficBytes = 4
			pauseMsg.TrafficClass = "memcontrolprotocol.Req"

			controlPort.Deliver(pauseMsg)

			madeProgress := tlbCtrlMW.handleIncomingCommands()

			Expect(madeProgress).To(BeTrue())
			nextState := &tlbComp.State
			Expect(nextState.TLBState).To(Equal(tlbStatePause))
		})

		It("should handle enable ctrl msg after pause", func() {
			pause := memcontrolprotocol.Req{
				Command: memcontrolprotocol.CmdPause,
			}
			pause.ID = timing.GetIDGenerator().Generate()
			pause.Src = messaging.RemotePort("Agent")
			pause.Dst = controlPort.AsRemote()
			pause.TrafficBytes = 4
			pause.TrafficClass = "memcontrolprotocol.Req"

			controlPort.Deliver(pause)

			madeProgress := tlbCtrlMW.handleIncomingCommands()

			Expect(madeProgress).To(BeTrue())
			nextState := &tlbComp.State
			Expect(nextState.TLBState).To(Equal(tlbStatePause))

			// Drain the Pause ack so the next ack has room in the
			// single-slot control port outgoing buffer.
			Expect(controlPort.RetrieveOutgoing()).
				To(BeAssignableToTypeOf(memcontrolprotocol.Rsp{}))

			enable := memcontrolprotocol.Req{
				Command: memcontrolprotocol.CmdEnable,
			}
			enable.ID = timing.GetIDGenerator().Generate()
			enable.Src = messaging.RemotePort("Agent")
			enable.Dst = controlPort.AsRemote()
			enable.TrafficBytes = 4
			enable.TrafficClass = "memcontrolprotocol.Req"

			controlPort.Deliver(enable)

			madeProgress = tlbCtrlMW.handleIncomingCommands()
			Expect(madeProgress).To(BeTrue())
			nextState = &tlbComp.State
			Expect(nextState.TLBState).To(Equal(tlbStateEnable))
		})

		It("should handle drain ctrl msg", func() {
			drainMsg := memcontrolprotocol.Req{
				Command: memcontrolprotocol.CmdDrain,
			}
			drainMsg.ID = timing.GetIDGenerator().Generate()
			drainMsg.Src = messaging.RemotePort("Agent")
			drainMsg.Dst = controlPort.AsRemote()
			drainMsg.TrafficBytes = 4
			drainMsg.TrafficClass = "memcontrolprotocol.Req"

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
		tlbComp    *Comp
		lowModule  *idealEndpoint
		agent      *idealEndpoint
		connection messaging.Connection
		page       vm.Page
	)

	BeforeEach(func() {
		engine = timing.NewSerialEngine()

		connection = makeDirectConnection(engine)

		lowModule = newIdealEndpoint("LowModule")
		agent = newIdealEndpoint("Agent")

		reg := modeling.NewStandaloneRegistrar(engine)
		tlbComp = MakeBuilder().
			WithRegistrar(reg).
			WithSpec(DefaultSpec()).
			WithResources(Resources{
				TranslationProviderMapper: &mem.SinglePortMapper{
					Port: lowModule.port.AsRemote(),
				},
			}).
			Build("TLB")

		assignDefaultPorts(reg, tlbComp)

		connection.PlugIn(agent.port)
		connection.PlugIn(lowModule.port)
		connection.PlugIn(tlbComp.GetPortByName("Top"))
		connection.PlugIn(tlbComp.GetPortByName("Bottom"))
		connection.PlugIn(tlbComp.GetPortByName("Control"))

		page = vm.Page{
			PID:   1,
			VAddr: 0x1000,
			PAddr: 0x2000,
			Valid: true,
		}

		// lowModule answers every translation request with the page.
		lowModule.onDeliver = func(msg messaging.Msg) {
			translationReq := msg.(vmprotocol.TranslationReq)
			rsp := vmprotocol.TranslationRsp{Page: page}
			rsp.ID = timing.GetIDGenerator().Generate()
			rsp.Src = lowModule.port.AsRemote()
			rsp.Dst = translationReq.Src
			rsp.RspTo = translationReq.ID
			rsp.TrafficClass = "vmprotocol.TranslationRsp"
			lowModule.port.Send(rsp)
		}
	})

	It("should do tlb miss", func() {
		req := vmprotocol.TranslationReq{}
		req.ID = timing.GetIDGenerator().Generate()
		req.Src = agent.port.AsRemote()
		req.Dst = tlbComp.GetPortByName("Top").AsRemote()
		req.PID = 1
		req.VAddr = 0x1000
		req.DeviceID = 1
		req.TrafficClass = "vmprotocol.TranslationReq"
		agent.port.Send(req)

		engine.Run()

		rsp := agent.lastDelivered.(vmprotocol.TranslationRsp)
		Expect(rsp.Page).To(Equal(page))
	})

	It("should have faster hit than miss", func() {
		time1 := engine.CurrentTime()
		req := vmprotocol.TranslationReq{}
		req.ID = timing.GetIDGenerator().Generate()
		req.Src = agent.port.AsRemote()
		req.Dst = tlbComp.GetPortByName("Top").AsRemote()
		req.PID = 1
		req.VAddr = 0x1000
		req.DeviceID = 1
		req.TrafficClass = "vmprotocol.TranslationReq"
		agent.port.Send(req)

		engine.Run()

		rsp := agent.lastDelivered.(vmprotocol.TranslationRsp)
		Expect(rsp.Page).To(Equal(page))

		time2 := engine.CurrentTime()

		req2 := vmprotocol.TranslationReq{}
		req2.ID = timing.GetIDGenerator().Generate()
		req2.Src = agent.port.AsRemote()
		req2.Dst = tlbComp.GetPortByName("Top").AsRemote()
		req2.PID = 1
		req2.VAddr = 0x1000
		req2.DeviceID = 1
		req2.TrafficClass = "vmprotocol.TranslationReq"
		agent.port.Send(req2)

		engine.Run()

		rsp = agent.lastDelivered.(vmprotocol.TranslationRsp)
		Expect(rsp.Page).To(Equal(page))

		time3 := engine.CurrentTime()

		Expect(time3 - time2).To(BeNumerically("<", time2-time1))
	})
})
