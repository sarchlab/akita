package bottleneckanalysis

import (
	gomock "github.com/golang/mock/gomock"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"gitlab.com/akita/akita/v3/sim"
)

var _ = Describe("BufferAnalyzer", func() {
	var (
		mockCtrl       *gomock.Controller
		timeTeller     *MockTimeTeller
		bufferAnalyzer *BufferAnalyzer
	)

	BeforeEach(func() {
		mockCtrl = gomock.NewController(GinkgoT())
		timeTeller = NewMockTimeTeller(mockCtrl)

		bufferAnalyzer = MakeBufferAnalyzerBuilder().
			WithTimeTeller(timeTeller).
			Build()
	})

	AfterEach(func() {
		mockCtrl.Finish()
	})

	It("should calculate average buffer level", func() {
		timeTeller.EXPECT().CurrentTime().Return(sim.VTimeInSec(0.0))
		buf := bufferAnalyzer.CreateBuffer("Buf", 10)

		timeTeller.EXPECT().CurrentTime().Return(sim.VTimeInSec(0))
		timeTeller.EXPECT().CurrentTime().Return(sim.VTimeInSec(10))
		timeTeller.EXPECT().CurrentTime().Return(sim.VTimeInSec(20))
		timeTeller.EXPECT().CurrentTime().Return(sim.VTimeInSec(30))
		timeTeller.EXPECT().CurrentTime().Return(sim.VTimeInSec(40))

		buf.Push(1)
		buf.Push(1)
		buf.Push(1)
		buf.Push(1)
		buf.Push(1)

		Expect(bufferAnalyzer.getBufAverageLevel("Buf")).To(Equal(float64(2.5)))
	})

	It("should calculate per-period buffer level", func() {
		bufferAnalyzer.period = 0.1
		bufferAnalyzer.usePeriod = true

		timeTeller.EXPECT().CurrentTime().Return(sim.VTimeInSec(0.0))
		buf := bufferAnalyzer.CreateBuffer("Buf", 10)

		timeTeller.EXPECT().CurrentTime().Return(sim.VTimeInSec(0))
		timeTeller.EXPECT().CurrentTime().Return(sim.VTimeInSec(0.049))
		timeTeller.EXPECT().CurrentTime().Return(sim.VTimeInSec(0.098))

		buf.Push(1)
		buf.Push(1)
		buf.Push(1)

		Expect(bufferAnalyzer.getInPeriodBufAverageLevel("Buf")).
			To(BeNumerically("~", 1.5, 0.01))
	})
})
