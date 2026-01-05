package gmmu

import (
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	gomock "go.uber.org/mock/gomock"

	"github.com/sarchlab/akita/v4/mem/vm"
	"github.com/sarchlab/akita/v4/sim"
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
		gmmu               *GMMU
		// mmuMiddleware *middleware
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

		bottomPort = NewMockPort(mockCtrl)
		bottomPort.EXPECT().AsRemote().
			Return(sim.RemotePort("BottomPort")).
			AnyTimes()

		builder := MakeBuilder().
			WithEngine(engine).
			WithDeviceID(0).
			WithPageWalkingLatency(1)

		gmmu = builder.Build("MMU")
		gmmu.topPort = topPort
		gmmu.bottomPort = bottomPort
		gmmu.pageTable = pageTable
	})

	AfterEach(func() {
		mockCtrl.Finish()
	})

	Context("GMMU Builder", func() {
		It("should build GMMU correctly", func() {
			Expect(gmmu.Engine).To(Equal(engine))
			Expect(gmmu.Freq).To(Equal(1 * sim.GHz))
			Expect(gmmu.maxRequestsInFlight).To(Equal(16))
			Expect(gmmu.pageTable).To(Equal(pageTable))
			Expect(gmmu.topPort).To(Equal(topPort))
			Expect(gmmu.deviceID).To(Equal(uint64(0)))
		})
	})

	Context("GMMU parse from top", func() {
		It("should process translation request", func() {
			req := vm.TranslationReqBuilder{}.
				WithDst(gmmu.topPort.AsRemote()).
				WithPID(1).
				WithVAddr(0x00000001).
				WithDeviceID(0).
				Build()

			topPort.EXPECT().
				RetrieveIncoming().
				Return(req)

			topPort.EXPECT().CanSend().
				Return(false)

			gmmu.Tick()

			Expect(len(gmmu.walkingTranslations)).To(Equal(1))
		})

		It("should walk page table", func() {
			req := vm.TranslationReqBuilder{}.
				WithDst(gmmu.topPort.AsRemote()).
				WithSrc(upperComponentPort.AsRemote()).
				WithPID(1).
				WithVAddr(0x10000001).
				WithDeviceID(0).
				Build()

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

			var sentRsp sim.Msg
			topPort.EXPECT().
				Send(gomock.Any()).
				Do(func(msg sim.Msg) {
					sentRsp = msg
				}).
				Return(nil)

			gmmu.parseFromTop()
			gmmu.walkPageTable()
			gmmu.walkPageTable()

			Expect(sentRsp).NotTo(BeNil())
			translationRsp := sentRsp.(*vm.TranslationRsp)
			Expect(translationRsp.Page).To(Equal(page))
			Expect(translationRsp.Page.PID).To(Equal(req.PID))
		})

	})
})
