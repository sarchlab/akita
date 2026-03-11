package writeevict

import (
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/sarchlab/akita/v5/mem/cache"
	"github.com/sarchlab/akita/v5/modeling"
	"github.com/sarchlab/akita/v5/sim"
	"go.uber.org/mock/gomock"
)

var _ = Describe("Control Stage", func() {

	var (
		mockCtrl     *gomock.Controller
		ctrlPort     *MockPort
		topPort      *MockPort
		bottomPort   *MockPort
		transactions []*transaction
		directory    *MockDirectory
		s            *controlStage
		cacheComp    *Comp
		inBuf        *MockBuffer
		mshr         *MockMSHR
		c            *coalescer
	)

	BeforeEach(func() {
		mockCtrl = gomock.NewController(GinkgoT())

		ctrlPort = NewMockPort(mockCtrl)
		ctrlPort.EXPECT().
			AsRemote().
			Return(sim.RemotePort("CtrlPort")).
			AnyTimes()
		topPort = NewMockPort(mockCtrl)
		topPort.EXPECT().
			AsRemote().
			Return(sim.RemotePort("TopPort")).
			AnyTimes()
		bottomPort = NewMockPort(mockCtrl)
		bottomPort.EXPECT().
			AsRemote().
			Return(sim.RemotePort("BottomPort")).
			AnyTimes()

		directory = NewMockDirectory(mockCtrl)
		inBuf = NewMockBuffer(mockCtrl)
		mshr = NewMockMSHR(mockCtrl)
		c = &coalescer{cache: cacheComp}

		transactions = nil

		cacheComp = &Comp{
			topPort:               topPort,
			bottomPort:            bottomPort,
			dirBuf:                inBuf,
			mshr:                  mshr,
			coalesceStage:         c,
			maxNumConcurrentTrans: 32,
		}
		cacheComp.Component = modeling.NewBuilder[Spec, State]().
			WithEngine(nil).
			WithFreq(1 * sim.GHz).
			WithSpec(Spec{}).
			Build("Cache")

		s = &controlStage{
			ctrlPort:     ctrlPort,
			transactions: &transactions,
			directory:    directory,
			cache:        cacheComp,
		}
	})

	AfterEach(func() {
		mockCtrl.Finish()
	})

	It("should do nothing if no request", func() {
		ctrlPort.EXPECT().PeekIncoming().Return(nil)

		madeProgress := s.Tick()

		Expect(madeProgress).To(BeFalse())
	})

	It("should wait for the cache to finish transactions", func() {
		transactions = []*transaction{{}}
		s.cache.transactions = transactions
		flushReq := &cache.FlushReq{}
		flushReq.ID = sim.GetIDGenerator().Generate()
		flushReq.TrafficBytes = 0
		flushReq.TrafficClass = "ctrl"
		flushReq.DiscardInflight = false
		s.currFlushReq = flushReq
		ctrlPort.EXPECT().PeekIncoming().Return(flushReq)

		madeProgress := s.Tick()

		Expect(madeProgress).To(BeFalse())
	})

	It("should reset directory", func() {
		flushReq := &cache.FlushReq{}
		flushReq.ID = sim.GetIDGenerator().Generate()
		flushReq.InvalidateAllCachelines = true
		flushReq.DiscardInflight = true
		flushReq.PauseAfterFlushing = true
		flushReq.TrafficBytes = 0
		flushReq.TrafficClass = "ctrl"
		s.currFlushReq = flushReq
		ctrlPort.EXPECT().Send(gomock.Any()).Do(func(msg sim.Msg) {
			Expect(msg.Meta().RspTo).To(Equal(flushReq.ID))
		})

		topPort.EXPECT().PeekIncoming().Return(nil)
		bottomPort.EXPECT().PeekIncoming().Return(nil)
		inBuf.EXPECT().Pop()
		directory.EXPECT().Reset()
		mshr.EXPECT().Reset()

		ctrlPort.EXPECT().PeekIncoming().Return(flushReq)

		madeProgress := s.Tick()

		Expect(madeProgress).To(BeTrue())
		Expect(s.currFlushReq).To(BeNil())
	})

})
