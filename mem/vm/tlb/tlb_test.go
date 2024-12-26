package tlb

import (
	"github.com/golang/mock/gomock"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/sarchlab/akita/v4/mem/vm"
	"github.com/sarchlab/akita/v4/mem/vm/tlb/internal"
	"github.com/sarchlab/akita/v4/noc/directconnection"
	"github.com/sarchlab/akita/v4/sim/id"
	"github.com/sarchlab/akita/v4/sim/modeling"
	"github.com/sarchlab/akita/v4/sim/timing"
)

var _ = Describe("TLB", func() {
	var (
		mockCtrl      *gomock.Controller
		engine        *MockEngine
		simulation    *MockSimulation
		tlb           *Comp
		tlbMiddleware *middleware
		set           *MockSet
		topPort       *MockPort
		bottomPort    *MockPort
		controlPort   *MockPort
	)

	BeforeEach(func() {
		mockCtrl = gomock.NewController(GinkgoT())
		engine = NewMockEngine(mockCtrl)

		simulation = NewMockSimulation(mockCtrl)
		simulation.EXPECT().GetEngine().Return(engine).AnyTimes()
		simulation.EXPECT().
			RegisterStateHolder(gomock.Any()).
			Return().
			AnyTimes()

		set = NewMockSet(mockCtrl)
		topPort = NewMockPort(mockCtrl)
		topPort.EXPECT().
			AsRemote().
			Return(modeling.RemotePort("TopPort")).
			AnyTimes()

		bottomPort = NewMockPort(mockCtrl)
		bottomPort.EXPECT().
			AsRemote().
			Return(modeling.RemotePort("BottomPort")).
			AnyTimes()

		controlPort = NewMockPort(mockCtrl)
		controlPort.EXPECT().
			AsRemote().
			Return(modeling.RemotePort("ControlPort")).
			AnyTimes()

		tlb = MakeBuilder().WithSimulation(simulation).Build("TLB")
		tlb.topPort = topPort
		tlb.bottomPort = bottomPort
		tlb.controlPort = controlPort
		tlb.Sets = []internal.Set{set}

		tlbMiddleware = tlb.Middlewares()[0].(*middleware)
	})

	AfterEach(func() {
		mockCtrl.Finish()
	})

	It("should do nothing if there is no req in TopPort", func() {
		topPort.EXPECT().PeekIncoming().Return(nil)

		madeProgress := tlbMiddleware.lookup()

		Expect(madeProgress).To(BeFalse())
	})

	Context("hit", func() {
		var (
			wayID int
			page  vm.Page
			req   vm.TranslationReq
		)

		BeforeEach(func() {
			wayID = 1
			page = vm.Page{
				PID:   1,
				VAddr: 0x100,
				PAddr: 0x200,
				Valid: true,
			}
			set.EXPECT().Lookup(vm.PID(1), uint64(0x100)).
				Return(wayID, page, true)

			req = vm.TranslationReq{
				MsgMeta: modeling.MsgMeta{
					ID:  id.Generate(),
					Src: modeling.RemotePort(""),
					Dst: modeling.RemotePort("TLB"),
				},
				PID:      1,
				VAddr:    0x100,
				DeviceID: 1,
			}
		})

		It("should respond to top", func() {
			topPort.EXPECT().PeekIncoming().Return(req)
			topPort.EXPECT().RetrieveIncoming()
			topPort.EXPECT().Send(gomock.Any())

			set.EXPECT().Visit(wayID)

			madeProgress := tlbMiddleware.lookup()

			Expect(madeProgress).To(BeTrue())
		})

		It("should stall if cannot send to top", func() {
			topPort.EXPECT().PeekIncoming().Return(req)
			topPort.EXPECT().Send(gomock.Any()).
				Return(&modeling.SendError{})

			madeProgress := tlbMiddleware.lookup()

			Expect(madeProgress).To(BeFalse())
		})
	})

	Context("miss", func() {
		var (
			wayID int
			page  vm.Page
			req   vm.TranslationReq
		)

		BeforeEach(func() {
			wayID = 1
			page = vm.Page{
				PID:   1,
				VAddr: 0x100,
				PAddr: 0x200,
				Valid: false,
			}
			set.EXPECT().
				Lookup(vm.PID(1), uint64(0x100)).
				Return(wayID, page, true).
				AnyTimes()

			req = vm.TranslationReq{
				MsgMeta: modeling.MsgMeta{
					ID:  id.Generate(),
					Src: modeling.RemotePort(""),
					Dst: modeling.RemotePort("TLB"),
				},
				PID:      1,
				VAddr:    0x100,
				DeviceID: 1,
			}
		})

		It("should fetch from bottom and add entry to MSHR", func() {
			topPort.EXPECT().PeekIncoming().Return(req)
			topPort.EXPECT().RetrieveIncoming()
			bottomPort.EXPECT().Send(gomock.Any()).
				Do(func(req vm.TranslationReq) {
					Expect(req.VAddr).To(Equal(uint64(0x100)))
					Expect(req.PID).To(Equal(vm.PID(1)))
					Expect(req.DeviceID).To(Equal(uint64(1)))
				}).
				Return(nil)

			madeProgress := tlbMiddleware.lookup()

			Expect(madeProgress).To(BeTrue())
			Expect(tlb.mshr.IsEntryPresent(vm.PID(1), uint64(0x100))).
				To(Equal(true))
		})

		It("should find the entry in MSHR and not request from bottom", func() {
			tlb.mshr.Add(1, 0x100)
			topPort.EXPECT().PeekIncoming().Return(req)
			topPort.EXPECT().RetrieveIncoming()

			madeProgress := tlbMiddleware.lookup()
			Expect(tlb.mshr.IsEntryPresent(vm.PID(1), uint64(0x100))).
				To(Equal(true))
			Expect(madeProgress).To(BeTrue())
		})

		It("should stall if bottom is busy", func() {
			topPort.EXPECT().PeekIncoming().Return(req)
			bottomPort.EXPECT().Send(gomock.Any()).
				Return(&modeling.SendError{})

			madeProgress := tlbMiddleware.lookup()

			Expect(madeProgress).To(BeFalse())
		})
	})

	Context("parse bottom", func() {
		var (
			wayID       int
			req         vm.TranslationReq
			fetchBottom vm.TranslationReq
			page        vm.Page
			rsp         vm.TranslationRsp
		)

		BeforeEach(func() {
			wayID = 1
			req = vm.TranslationReq{
				MsgMeta: modeling.MsgMeta{
					ID:  id.Generate(),
					Src: modeling.RemotePort(""),
					Dst: modeling.RemotePort("TLB"),
				},
				PID:      1,
				VAddr:    0x100,
				DeviceID: 1,
			}
			fetchBottom = vm.TranslationReq{
				MsgMeta: modeling.MsgMeta{
					ID:  id.Generate(),
					Src: modeling.RemotePort(""),
					Dst: modeling.RemotePort("Bottom"),
				},
				PID:      1,
				VAddr:    0x100,
				DeviceID: 1,
			}
			page = vm.Page{
				PID:   1,
				VAddr: 0x100,
				PAddr: 0x200,
				Valid: true,
			}
			rsp = vm.TranslationRsp{
				MsgMeta: modeling.MsgMeta{
					ID:  id.Generate(),
					Src: modeling.RemotePort("Bottom"),
					Dst: modeling.RemotePort("TLB"),
				},
				RespondTo: fetchBottom.ID,
				Page:      page,
			}
		})

		It("should do nothing if no return", func() {
			bottomPort.EXPECT().PeekIncoming().Return(nil)

			madeProgress := tlbMiddleware.parseBottom()

			Expect(madeProgress).To(BeFalse())
		})

		It("should stall if the TLB is responding to an MSHR entry", func() {
			mshrEntry := tlb.mshr.Add(1, 0x100)
			mshrEntry.Requests = append(mshrEntry.Requests, req)
			tlb.respondingMSHREntry = mshrEntry

			madeProgress := tlbMiddleware.parseBottom()

			Expect(madeProgress).To(BeFalse())
		})

		It("should parse respond from bottom", func() {
			bottomPort.EXPECT().PeekIncoming().Return(rsp)
			bottomPort.EXPECT().RetrieveIncoming()
			mshrEntry := tlb.mshr.Add(1, 0x100)
			mshrEntry.Requests = append(mshrEntry.Requests, req)
			mshrEntry.reqToBottom = vm.TranslationReq{}

			set.EXPECT().Evict().Return(wayID, true)
			set.EXPECT().Update(wayID, page)
			set.EXPECT().Visit(wayID)

			// topPort.EXPECT().Send(gomock.Any()).
			// 	Do(func(rsp *vm.TranslationRsp) {
			// 		Expect(rsp.Page).To(Equal(page))
			// 		Expect(rsp.RespondTo).To(Equal(req.ID))
			// 	})

			madeProgress := tlbMiddleware.parseBottom()

			Expect(madeProgress).To(BeTrue())
			Expect(tlb.respondingMSHREntry).NotTo(BeNil())
			Expect(tlb.mshr.IsEntryPresent(vm.PID(1), uint64(0x100))).
				To(Equal(false))
		})

		It("should respond", func() {
			mshrEntry := tlb.mshr.Add(1, 0x100)
			mshrEntry.Requests = append(mshrEntry.Requests, req)
			tlb.respondingMSHREntry = mshrEntry

			topPort.EXPECT().Send(gomock.Any()).Return(nil)

			madeProgress := tlbMiddleware.respondMSHREntry()

			Expect(madeProgress).To(BeTrue())
			Expect(mshrEntry.Requests).To(HaveLen(0))
			Expect(tlb.respondingMSHREntry).To(BeNil())
		})
	})

	Context("flush related handling", func() {
		var (
		// flushReq   *TLBFlushReq
		// restartReq *TLBRestartReq
		)

		BeforeEach(func() {

			// restartReq = TLBRestartReqBuilder{}.
			// 	WithSrc(nil).
			// 	WithDst(nil).
			// 	WithSendTime(10).
			// 	Build()
		})

		It("should do nothing if no req", func() {
			controlPort.EXPECT().PeekIncoming().Return(nil)
			madeProgress := tlbMiddleware.performCtrlReq()
			Expect(madeProgress).To(BeFalse())
		})

		It("should handle flush request", func() {
			flushReq := FlushReq{
				MsgMeta: modeling.MsgMeta{
					ID:  id.Generate(),
					Src: modeling.RemotePort(""),
					Dst: controlPort.AsRemote(),
				},
				VAddrs: []uint64{0x1000},
				PID:    1,
			}
			page := vm.Page{
				PID:   1,
				VAddr: 0x1000,
				Valid: true,
			}
			wayID := 1

			set.EXPECT().Lookup(vm.PID(1), uint64(0x1000)).
				Return(wayID, page, true)
			set.EXPECT().Update(wayID, vm.Page{
				PID:   1,
				VAddr: 0x1000,
				Valid: false,
			})
			controlPort.EXPECT().PeekIncoming().Return(flushReq)
			controlPort.EXPECT().RetrieveIncoming().Return(flushReq)
			controlPort.EXPECT().Send(gomock.Any())

			madeProgress := tlbMiddleware.performCtrlReq()

			Expect(madeProgress).To(BeTrue())
			Expect(tlb.isPaused).To(BeTrue())
		})

		It("should handle restart request", func() {
			restartReq := RestartReq{
				MsgMeta: modeling.MsgMeta{
					ID:  id.Generate(),
					Src: modeling.RemotePort(""),
					Dst: controlPort.AsRemote(),
				},
			}
			controlPort.EXPECT().PeekIncoming().
				Return(restartReq)
			controlPort.EXPECT().RetrieveIncoming().
				Return(restartReq)
			controlPort.EXPECT().Send(gomock.Any())
			topPort.EXPECT().RetrieveIncoming().Return(nil)
			bottomPort.EXPECT().RetrieveIncoming().Return(nil)

			madeProgress := tlbMiddleware.performCtrlReq()

			Expect(madeProgress).To(BeTrue())
			Expect(tlb.isPaused).To(BeFalse())
		})
	})
})

var _ = Describe("TLB Integration", func() {
	var (
		mockCtrl   *gomock.Controller
		engine     timing.Engine
		simulation *MockSimulation
		tlb        *Comp
		lowModule  *MockPort
		agent      *MockPort
		connection modeling.Connection
		page       vm.Page
	)

	BeforeEach(func() {
		mockCtrl = gomock.NewController(GinkgoT())
		engine = timing.NewSerialEngine()

		simulation = NewMockSimulation(mockCtrl)
		simulation.EXPECT().GetEngine().Return(engine).AnyTimes()
		simulation.EXPECT().
			RegisterStateHolder(gomock.Any()).
			Return().
			AnyTimes()

		lowModule = NewMockPort(mockCtrl)
		lowModule.EXPECT().
			AsRemote().
			Return(modeling.RemotePort("LowModule")).
			AnyTimes()
		lowModuleCall := lowModule.EXPECT().
			PeekOutgoing().
			Return(nil).
			AnyTimes()

		agent = NewMockPort(mockCtrl)
		agent.EXPECT().PeekOutgoing().Return(nil).AnyTimes()
		agent.EXPECT().
			AsRemote().
			Return(modeling.RemotePort("Agent")).
			AnyTimes()

		connection = directconnection.MakeBuilder().
			WithEngine(engine).
			WithFreq(1 * timing.GHz).
			Build("Conn")
		tlb = MakeBuilder().
			WithSimulation(simulation).
			Build("TLB")
		tlb.LowModule = lowModule.AsRemote()

		agent.EXPECT().SetConnection(connection)
		lowModule.EXPECT().SetConnection(connection)
		connection.PlugIn(agent)
		connection.PlugIn(lowModule)
		connection.PlugIn(tlb.topPort)
		connection.PlugIn(tlb.bottomPort)
		connection.PlugIn(tlb.controlPort)

		page = vm.Page{
			PID:   1,
			VAddr: 0x1000,
			PAddr: 0x2000,
			Valid: true,
		}
		lowModule.EXPECT().Deliver(gomock.Any()).
			Do(func(req vm.TranslationReq) {
				rsp := vm.TranslationRsp{
					MsgMeta: modeling.MsgMeta{
						ID:  id.Generate(),
						Src: lowModule.AsRemote(),
						Dst: req.Src,
					},
					RespondTo: req.ID,
					Page:      page,
				}
				lowModuleCall.Times(0)
				lowModule.EXPECT().PeekOutgoing().Return(rsp)
				lowModule.EXPECT().RetrieveOutgoing().Return(rsp)
				lowModule.EXPECT().PeekOutgoing().Return(nil).AnyTimes()
			}).
			AnyTimes()
	})

	AfterEach(func() {
		mockCtrl.Finish()
	})

	It("should do tlb miss", func() {
		req := vm.TranslationReq{
			MsgMeta: modeling.MsgMeta{
				ID:  id.Generate(),
				Src: agent.AsRemote(),
				Dst: tlb.topPort.AsRemote(),
			},
			PID:      1,
			VAddr:    0x1000,
			DeviceID: 1,
		}
		tlb.topPort.Deliver(req)

		agent.EXPECT().Deliver(gomock.Any()).
			Do(func(rsp vm.TranslationRsp) {
				Expect(rsp.Page).To(Equal(page))
			})

		engine.Run()
	})

	It("should have faster hit than miss", func() {
		time1 := engine.Now()
		req := vm.TranslationReq{
			MsgMeta: modeling.MsgMeta{
				ID:  id.Generate(),
				Src: agent.AsRemote(),
				Dst: tlb.topPort.AsRemote(),
			},
			PID:      1,
			VAddr:    0x1000,
			DeviceID: 1,
		}
		tlb.topPort.Deliver(req)

		agent.EXPECT().Deliver(gomock.Any()).
			Do(func(rsp vm.TranslationRsp) {
				Expect(rsp.Page).To(Equal(page))
			})

		engine.Run()

		time2 := engine.Now()

		tlb.topPort.Deliver(req)

		agent.EXPECT().Deliver(gomock.Any()).
			Do(func(rsp vm.TranslationRsp) {
				Expect(rsp.Page).To(Equal(page))
			})

		engine.Run()

		time3 := engine.Now()

		Expect(time3 - time2).To(BeNumerically("<", time2-time1))
	})

	/*It("should have miss after shootdown ", func() {
		time1 := timing.VTimeInSec(10)
		req := vm.NewTranslationReq(time1, agent, tlb.TopPort, 1, 0x1000, 1)
		req.SetRecvTime(time1)
		tlb.TopPort.Recv(*req)
		agent.EXPECT().Recv(gomock.Any()).
			Do(func(rsp vm.TranslationReadyRsp) {
				Expect(rsp.Page).To(Equal(&page))
			})
		engine.Run()

		time2 := engine.Now()
		shootdownReq := vm.NewPTEInvalidationReq(
			time2, agent, tlb.ControlPort, 1, []uint64{0x1000})
		shootdownReq.SetRecvTime(time2)
		tlb.ControlPort.Recv(*shootdownReq)
		agent.EXPECT().Recv(gomock.Any()).
			Do(func(rsp vm.InvalidationCompleteRsp) {
				Expect(rsp.RespondTo).To(Equal(shootdownReq.ID))
			})
		engine.Run()

		time3 := engine.Now()
		req.SetRecvTime(time3)
		tlb.TopPort.Recv(*req)
		agent.EXPECT().Recv(gomock.Any()).
			Do(func(rsp vm.TranslationReadyRsp) {
				Expect(rsp.Page).To(Equal(&page))
			})
		engine.Run()
		time4 := engine.Now()

		Expect(time4 - time3).To(BeNumerically("~", time2-time1))
	})*/

})
