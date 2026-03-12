package gmmu

import (
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	gomock "go.uber.org/mock/gomock"

	"github.com/sarchlab/akita/v5/mem/vm"
	"github.com/sarchlab/akita/v5/modeling"
	"github.com/sarchlab/akita/v5/sim"
)

var _ = Describe("Builder", func() {

	var (
		mockCtrl           *gomock.Controller
		engine             *MockEngine
		upperComponentPort *MockPort
		lowerComponentPort *MockPort
		topPort            *MockPort
		bottomPort         *MockPort
		pageTable          *MockPageTable
		gmmuComp           *modeling.Component[Spec, State]
		mw                 *middleware
	)

	BeforeEach(func() {
		mockCtrl = gomock.NewController(GinkgoT())

		engine = NewMockEngine(mockCtrl)
		pageTable = NewMockPageTable(mockCtrl)

		upperComponentPort = NewMockPort(mockCtrl)
		upperComponentPort.EXPECT().AsRemote().
			Return(sim.RemotePort("UpperComponentPort")).
			AnyTimes()

		lowerComponentPort = NewMockPort(mockCtrl)
		lowerComponentPort.EXPECT().AsRemote().
			Return(sim.RemotePort("LowerComponentPort")).
			AnyTimes()

		topPort = NewMockPort(mockCtrl)
		topPort.EXPECT().AsRemote().
			Return(sim.RemotePort("TopPort")).
			AnyTimes()
		topPort.EXPECT().SetComponent(gomock.Any()).AnyTimes()

		bottomPort = NewMockPort(mockCtrl)
		bottomPort.EXPECT().AsRemote().
			Return(sim.RemotePort("BottomPort")).
			AnyTimes()
		bottomPort.EXPECT().SetComponent(gomock.Any()).AnyTimes()

		builder := MakeBuilder().
			WithEngine(engine).
			WithDeviceID(0).
			WithPageWalkingLatency(1).
			WithLowModule(lowerComponentPort.AsRemote()).
			WithPageTable(pageTable).
			WithTopPort(topPort).
			WithBottomPort(bottomPort)

		gmmuComp = builder.Build("MMU")
		mw = gmmuComp.Middlewares()[0].(*middleware)
	})

	AfterEach(func() {
		mockCtrl.Finish()
	})

	Context("GMMU Builder", func() {
		It("should build GMMU correctly", func() {
			Expect(gmmuComp.Engine).To(Equal(engine))
			Expect(gmmuComp.Freq).To(Equal(1 * sim.GHz))
			Expect(gmmuComp.GetSpec().MaxRequestsInFlight).To(Equal(16))
			Expect(mw.pageTable).To(Equal(pageTable))
			Expect(gmmuComp.GetSpec().DeviceID).To(Equal(uint64(0)))
		})
	})

	Context("GMMU parse from top", func() {
		It("should process translation request", func() {
			req := &vm.TranslationReq{}
			req.ID = sim.GetIDGenerator().Generate()
			req.Dst = topPort.AsRemote()
			req.PID = 1
			req.VAddr = 0x00000001
			req.DeviceID = 0
			req.TrafficClass = "vm.TranslationReq"

			topPort.EXPECT().
				RetrieveIncoming().
				Return(req)

			topPort.EXPECT().CanSend().
				Return(false)

			mw.Tick()

			nextState := gmmuComp.GetNextState()
			Expect(len(nextState.WalkingTranslations)).To(Equal(1))
		})

		It("should walk page table", func() {
			req := &vm.TranslationReq{}
			req.ID = sim.GetIDGenerator().Generate()
			req.Dst = topPort.AsRemote()
			req.Src = upperComponentPort.AsRemote()
			req.PID = 1
			req.VAddr = 0x10000001
			req.DeviceID = 0
			req.TrafficClass = "vm.TranslationReq"

			page := vm.Page{
				PID:      vm.PID(1),
				VAddr:    0x10000001,
				DeviceID: 0,
			}

			topPort.EXPECT().
				RetrieveIncoming().
				Return(req)

			topPort.EXPECT().
				RetrieveIncoming().
				Return(nil).
				AnyTimes()

			pageTable.EXPECT().
				Find(vm.PID(1), uint64(0x10000001)).
				Return(page, true).AnyTimes()

			topPort.EXPECT().CanSend().
				Return(true).
				AnyTimes()

			bottomPort.EXPECT().
				RetrieveIncoming().
				Return(nil).
				AnyTimes()

			var sentRsp *vm.TranslationRsp
			topPort.EXPECT().
				Send(gomock.Any()).
				Do(func(msg sim.Msg) {
					sentRsp = msg.(*vm.TranslationRsp)
				}).
				Return(nil)

			// Tick 1: parseFromTop adds translation to next; walkPageTable
			// sees empty cur. After tick, next becomes cur.
			gmmuComp.Tick()
			// Tick 2: walkPageTable sees the translation in cur, decrements
			// CycleLeft (latency=1 → 0).
			gmmuComp.Tick()
			// Tick 3: CycleLeft==0, page walk completes and sends response.
			gmmuComp.Tick()

			Expect(sentRsp).NotTo(BeNil())
			Expect(sentRsp.Page).To(Equal(page))
			Expect(sentRsp.Page.PID).To(Equal(req.PID))
		})

		It("should send request remotely", func() {
			req := &vm.TranslationReq{}
			req.ID = sim.GetIDGenerator().Generate()
			req.Dst = topPort.AsRemote()
			req.Src = upperComponentPort.AsRemote()
			req.PID = 1
			req.VAddr = 0x10000001
			req.DeviceID = 0
			req.TrafficClass = "vm.TranslationReq"

			page := vm.Page{
				PID:      vm.PID(1),
				VAddr:    0x10000001,
				DeviceID: 1,
			}

			topPort.EXPECT().
				RetrieveIncoming().
				Return(req)

			topPort.EXPECT().
				RetrieveIncoming().
				Return(nil).
				AnyTimes()

			topPort.EXPECT().CanSend().
				Return(true).
				AnyTimes()

			pageTable.EXPECT().
				Find(vm.PID(1), uint64(0x10000001)).
				Return(page, true).AnyTimes()

			bottomPort.EXPECT().
				CanSend().
				Return(true).
				AnyTimes()

			bottomPort.EXPECT().
				Send(gomock.Any()).
				Return(nil)

			bottomPort.EXPECT().
				RetrieveIncoming().
				Return(nil).
				AnyTimes()

			// Tick 1: parseFromTop adds translation to next.
			gmmuComp.Tick()
			// Tick 2: walkPageTable sees translation, decrements CycleLeft.
			gmmuComp.Tick()
			// Tick 3: CycleLeft==0, page is remote, sends request to bottom.
			gmmuComp.Tick()
		})

		It("should return response from remote page table", func() {
			req := &vm.TranslationReq{}
			req.ID = sim.GetIDGenerator().Generate()
			req.Dst = topPort.AsRemote()
			req.Src = upperComponentPort.AsRemote()
			req.PID = 1
			req.VAddr = 0x10000001
			req.DeviceID = 0
			req.TrafficClass = "vm.TranslationReq"

			page := vm.Page{
				PID:      vm.PID(1),
				VAddr:    0x10000001,
				DeviceID: 1,
			}

			topPort.EXPECT().
				RetrieveIncoming().
				Return(req)

			topPort.EXPECT().
				RetrieveIncoming().
				Return(nil).
				AnyTimes()

			topPort.EXPECT().
				CanSend().
				Return(true).
				AnyTimes()

			pageTable.EXPECT().
				Find(vm.PID(1), uint64(0x10000001)).
				Return(page, true).AnyTimes()

			bottomPort.EXPECT().
				CanSend().
				Return(true).
				AnyTimes()

			var sentReqToBottom *vm.TranslationReq
			bottomPort.EXPECT().
				Send(gomock.Any()).
				Do(func(msg sim.Msg) {
					sentReqToBottom = msg.(*vm.TranslationReq)
				}).
				Return(nil)

			// Before the remote response arrives, return nil.
			bottomPort.EXPECT().
				RetrieveIncoming().
				Return(nil).
				Times(3)

			// Tick 1: parseFromTop adds translation to next.
			gmmuComp.Tick()
			// Tick 2: walkPageTable sees translation, decrements CycleLeft.
			gmmuComp.Tick()
			// Tick 3: CycleLeft==0, page is remote, sends request to bottom.
			gmmuComp.Tick()

			// Now set up the response from the bottom port.
			bottomPort.EXPECT().
				RetrieveIncoming().
				DoAndReturn(func() sim.Msg {
					rsp := &vm.TranslationRsp{
						Page: page,
					}
					rsp.ID = sim.GetIDGenerator().Generate()
					rsp.Src = gmmuComp.GetSpec().LowModule
					rsp.Dst = bottomPort.AsRemote()
					rsp.RspTo = sentReqToBottom.ID
					rsp.TrafficClass = "vm.TranslationRsp"
					return rsp
				})

			var sentRsp *vm.TranslationRsp
			topPort.EXPECT().
				Send(gomock.Any()).
				Do(func(msg sim.Msg) {
					sentRsp = msg.(*vm.TranslationRsp)
				}).
				Return(nil)

			// Tick 4: fetchFromBottom receives response, sends to top.
			gmmuComp.Tick()

			Expect(sentRsp).NotTo(BeNil())
			Expect(sentRsp.Page).To(Equal(page))
			Expect(sentRsp.Page.PID).To(Equal(req.PID))
		})
	})
})
