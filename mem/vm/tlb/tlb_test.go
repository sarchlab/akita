package tlb

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

		tlbComp = MakeBuilder().
			WithRegistrar(modeling.NewStandaloneRegistrar(engine)).
			WithSpec(spec).
			WithResources(Resources{
				TranslationProviderMapper: &mem.SinglePortMapper{
					Port: remotePort,
				},
			}).
			Build("TLB")

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
		req := vm.TranslationReq{}
		req.ID = timing.GetIDGenerator().Generate()
		req.Src = messaging.RemotePort("Agent")
		req.Dst = topPort.AsRemote()
		req.PID = 1
		req.VAddr = uint64(0x100)
		req.DeviceID = 1
		req.TrafficClass = "vm.TranslationReq"
		topPort.Deliver(req)

		madeProgress := tlbMW.insertIntoPipeline()

		Expect(madeProgress).To(BeTrue())
	})

	Context("hit", func() {
		var (
			req vm.TranslationReq
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

			req = vm.TranslationReq{}
			req.ID = timing.GetIDGenerator().Generate()
			req.Src = messaging.RemotePort("Agent")
			req.PID = 1
			req.VAddr = uint64(0x100)
			req.DeviceID = 1
			req.TrafficClass = "vm.TranslationReq"
		})

		It("should respond to top", func() {
			madeProgress := tlbMW.lookup(req)

			Expect(madeProgress).To(BeTrue())
			rsp := topPort.RetrieveOutgoing()
			Expect(rsp).To(BeAssignableToTypeOf(vm.TranslationRsp{}))
		})
	})

	Context("miss", func() {
		var (
			req vm.TranslationReq
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

			req = vm.TranslationReq{}
			req.ID = timing.GetIDGenerator().Generate()
			req.Src = messaging.RemotePort("Agent")
			req.PID = 1
			req.VAddr = 0x100
			req.DeviceID = 1
			req.TrafficClass = "vm.TranslationReq"
		})

		It("should fetch from bottom and add entry to MSHR", func() {
			madeProgress := tlbMW.lookup(req)

			Expect(madeProgress).To(BeTrue())
			nextState := &tlbComp.State
			Expect(mshrIsEntryPresent(nextState.MSHREntries, vm.PID(1), uint64(0x100))).
				To(Equal(true))

			sent := bottomPort.RetrieveOutgoing()
			Expect(sent).To(BeAssignableToTypeOf(vm.TranslationReq{}))
			sentMsg := sent.(vm.TranslationReq)
			Expect(sentMsg.VAddr).To(Equal(uint64(0x100)))
			Expect(sentMsg.PID).To(Equal(vm.PID(1)))
			Expect(sentMsg.DeviceID).To(Equal(uint64(1)))
			Expect(sentMsg.Dst).To(Equal(remotePort))
		})
	})

	Context("parse bottom", func() {
		var (
			req         vm.TranslationReq
			fetchBottom vm.TranslationReq
			page        vm.Page
			rsp         vm.TranslationRsp
		)

		BeforeEach(func() {
			req = vm.TranslationReq{}
			req.ID = timing.GetIDGenerator().Generate()
			req.Src = messaging.RemotePort("Agent")
			req.PID = 1
			req.VAddr = 0x100
			req.DeviceID = 1
			req.TrafficClass = "vm.TranslationReq"
			fetchBottom = vm.TranslationReq{}
			fetchBottom.ID = timing.GetIDGenerator().Generate()
			fetchBottom.PID = 1
			fetchBottom.VAddr = 0x100
			fetchBottom.DeviceID = 1
			fetchBottom.TrafficClass = "vm.TranslationReq"
			page = vm.Page{
				PID:   1,
				VAddr: 0x100,
				PAddr: 0x200,
				Valid: true,
			}
			rsp = vm.TranslationRsp{
				Page: page,
			}
			rsp.ID = timing.GetIDGenerator().Generate()
			rsp.Src = messaging.RemotePort("Agent")
			rsp.Dst = bottomPort.AsRemote()
			rsp.RspTo = fetchBottom.ID
			rsp.TrafficClass = "vm.TranslationRsp"
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
				Requests: []vm.TranslationReq{req},
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
				Requests: []vm.TranslationReq{req},
			}

			madeProgress := tlbMW.respondMSHREntry()

			Expect(madeProgress).To(BeTrue())
			nextState := &tlbComp.State
			Expect(nextState.RespondingMSHRData.Requests).To(HaveLen(0))
			Expect(nextState.HasRespondingMSHR).To(BeFalse())

			rsp := topPort.RetrieveOutgoing()
			Expect(rsp).To(BeAssignableToTypeOf(vm.TranslationRsp{}))
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

			invReq := mem.ControlReq{
				Command:   mem.CmdInvalidate,
				Addresses: []uint64{0x1000},
				PID:       1,
			}
			invReq.ID = timing.GetIDGenerator().Generate()
			invReq.Src = messaging.RemotePort("Agent")
			invReq.Dst = controlPort.AsRemote()
			invReq.TrafficClass = "mem.ControlReq"

			controlPort.Deliver(invReq)

			madeProgress := tlbCtrlMW.handleIncomingCommands()
			Expect(madeProgress).To(BeTrue())

			_, gotPage, found := setLookup(&next.Sets[0], 1, 0x1000)
			Expect(found && gotPage.Valid).To(BeFalse())

			rspMsg := controlPort.RetrieveOutgoing()
			Expect(rspMsg).To(BeAssignableToTypeOf(mem.ControlRsp{}))
			rsp := rspMsg.(mem.ControlRsp)
			Expect(rsp.Command).To(Equal(mem.CmdInvalidate))
			Expect(rsp.Success).To(BeTrue())
		})

		It("should reject Invalidate while enabled", func() {
			next := &tlbComp.State
			next.TLBState = tlbStateEnable

			invReq := mem.ControlReq{Command: mem.CmdInvalidate}
			invReq.ID = timing.GetIDGenerator().Generate()
			invReq.Src = messaging.RemotePort("Agent")
			invReq.Dst = controlPort.AsRemote()
			invReq.TrafficClass = "mem.ControlReq"

			controlPort.Deliver(invReq)
			Expect(tlbCtrlMW.handleIncomingCommands()).To(BeTrue())

			rspMsg := controlPort.RetrieveOutgoing()
			rsp := rspMsg.(mem.ControlRsp)
			Expect(rsp.Success).To(BeFalse())
			Expect(rsp.Error).To(Equal(control.ErrMustBePausedOrDrained))
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

			invReq := mem.ControlReq{Command: mem.CmdInvalidate, PID: 1}
			invReq.ID = timing.GetIDGenerator().Generate()
			invReq.Src = messaging.RemotePort("Agent")
			invReq.Dst = controlPort.AsRemote()
			invReq.TrafficClass = "mem.ControlReq"
			controlPort.Deliver(invReq)

			Expect(tlbCtrlMW.handleIncomingCommands()).To(BeTrue())

			// Only the PID-1 entry is dropped; the PID-2 entry survives.
			Expect(next.Sets[0].Blocks[0].Page.Valid).To(BeFalse())
			Expect(next.Sets[0].Blocks[1].Page.Valid).To(BeTrue())

			rsp := controlPort.RetrieveOutgoing().(mem.ControlRsp)
			Expect(rsp.Command).To(Equal(mem.CmdInvalidate))
			Expect(rsp.Success).To(BeTrue())
		})

		It("should handle restart request", func() {
			restartReq := mem.ControlReq{Command: mem.CmdReset}
			restartReq.ID = timing.GetIDGenerator().Generate()
			restartReq.Src = messaging.RemotePort("Agent")
			restartReq.Dst = controlPort.AsRemote()
			restartReq.TrafficClass = "mem.ControlReq"
			controlPort.Deliver(restartReq)

			madeProgress := tlbCtrlMW.handleIncomingCommands()

			Expect(madeProgress).To(BeTrue())

			rsp := controlPort.RetrieveOutgoing()
			Expect(rsp).To(BeAssignableToTypeOf(mem.ControlRsp{}))
		})
	})

	Context("other control signals", func() {
		It("should handle pause ctrl msg", func() {
			pauseMsg := mem.ControlReq{
				Command: mem.CmdPause,
			}
			pauseMsg.ID = timing.GetIDGenerator().Generate()
			pauseMsg.Src = messaging.RemotePort("Agent")
			pauseMsg.Dst = controlPort.AsRemote()
			pauseMsg.TrafficBytes = 4
			pauseMsg.TrafficClass = "mem.ControlReq"

			controlPort.Deliver(pauseMsg)

			madeProgress := tlbCtrlMW.handleIncomingCommands()

			Expect(madeProgress).To(BeTrue())
			nextState := &tlbComp.State
			Expect(nextState.TLBState).To(Equal(tlbStatePause))
		})

		It("should handle enable ctrl msg after pause", func() {
			pause := mem.ControlReq{
				Command: mem.CmdPause,
			}
			pause.ID = timing.GetIDGenerator().Generate()
			pause.Src = messaging.RemotePort("Agent")
			pause.Dst = controlPort.AsRemote()
			pause.TrafficBytes = 4
			pause.TrafficClass = "mem.ControlReq"

			controlPort.Deliver(pause)

			madeProgress := tlbCtrlMW.handleIncomingCommands()

			Expect(madeProgress).To(BeTrue())
			nextState := &tlbComp.State
			Expect(nextState.TLBState).To(Equal(tlbStatePause))

			// Drain the Pause ack so the next ack has room in the
			// single-slot control port outgoing buffer.
			Expect(controlPort.RetrieveOutgoing()).
				To(BeAssignableToTypeOf(mem.ControlRsp{}))

			enable := mem.ControlReq{
				Command: mem.CmdEnable,
			}
			enable.ID = timing.GetIDGenerator().Generate()
			enable.Src = messaging.RemotePort("Agent")
			enable.Dst = controlPort.AsRemote()
			enable.TrafficBytes = 4
			enable.TrafficClass = "mem.ControlReq"

			controlPort.Deliver(enable)

			madeProgress = tlbCtrlMW.handleIncomingCommands()
			Expect(madeProgress).To(BeTrue())
			nextState = &tlbComp.State
			Expect(nextState.TLBState).To(Equal(tlbStateEnable))
		})

		It("should handle drain ctrl msg", func() {
			drainMsg := mem.ControlReq{
				Command: mem.CmdDrain,
			}
			drainMsg.ID = timing.GetIDGenerator().Generate()
			drainMsg.Src = messaging.RemotePort("Agent")
			drainMsg.Dst = controlPort.AsRemote()
			drainMsg.TrafficBytes = 4
			drainMsg.TrafficClass = "mem.ControlReq"

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

		tlbComp = MakeBuilder().
			WithRegistrar(modeling.NewStandaloneRegistrar(engine)).
			WithSpec(DefaultSpec()).
			WithResources(Resources{
				TranslationProviderMapper: &mem.SinglePortMapper{
					Port: lowModule.port.AsRemote(),
				},
			}).
			Build("TLB")

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
			translationReq := msg.(vm.TranslationReq)
			rsp := vm.TranslationRsp{Page: page}
			rsp.ID = timing.GetIDGenerator().Generate()
			rsp.Src = lowModule.port.AsRemote()
			rsp.Dst = translationReq.Src
			rsp.RspTo = translationReq.ID
			rsp.TrafficClass = "vm.TranslationRsp"
			lowModule.port.Send(rsp)
		}
	})

	It("should do tlb miss", func() {
		req := vm.TranslationReq{}
		req.ID = timing.GetIDGenerator().Generate()
		req.Src = agent.port.AsRemote()
		req.Dst = tlbComp.GetPortByName("Top").AsRemote()
		req.PID = 1
		req.VAddr = 0x1000
		req.DeviceID = 1
		req.TrafficClass = "vm.TranslationReq"
		agent.port.Send(req)

		engine.Run()

		rsp := agent.lastDelivered.(vm.TranslationRsp)
		Expect(rsp.Page).To(Equal(page))
	})

	It("should have faster hit than miss", func() {
		time1 := engine.CurrentTime()
		req := vm.TranslationReq{}
		req.ID = timing.GetIDGenerator().Generate()
		req.Src = agent.port.AsRemote()
		req.Dst = tlbComp.GetPortByName("Top").AsRemote()
		req.PID = 1
		req.VAddr = 0x1000
		req.DeviceID = 1
		req.TrafficClass = "vm.TranslationReq"
		agent.port.Send(req)

		engine.Run()

		rsp := agent.lastDelivered.(vm.TranslationRsp)
		Expect(rsp.Page).To(Equal(page))

		time2 := engine.CurrentTime()

		req2 := vm.TranslationReq{}
		req2.ID = timing.GetIDGenerator().Generate()
		req2.Src = agent.port.AsRemote()
		req2.Dst = tlbComp.GetPortByName("Top").AsRemote()
		req2.PID = 1
		req2.VAddr = 0x1000
		req2.DeviceID = 1
		req2.TrafficClass = "vm.TranslationReq"
		agent.port.Send(req2)

		engine.Run()

		rsp = agent.lastDelivered.(vm.TranslationRsp)
		Expect(rsp.Page).To(Equal(page))

		time3 := engine.CurrentTime()

		Expect(time3 - time2).To(BeNumerically("<", time2-time1))
	})
})
