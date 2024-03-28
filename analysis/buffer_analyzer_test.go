package analysis

import (
	gomock "github.com/golang/mock/gomock"
	. "github.com/onsi/ginkgo/v2"

	// . "github.com/onsi/gomega"
	"github.com/sarchlab/akita/v3/sim"
)

var _ = Describe("BufferAnalyzer", func() {
	var (
		mockCtrl       *gomock.Controller
		timeTeller     *MockTimeTeller
		logger         *MockPerfLogger
		buffer         *MockBuffer
		bufferAnalyzer *BufferAnalyzer
	)

	BeforeEach(func() {
		mockCtrl = gomock.NewController(GinkgoT())
		timeTeller = NewMockTimeTeller(mockCtrl)
		logger = NewMockPerfLogger(mockCtrl)
		buffer = NewMockBuffer(mockCtrl)
		buffer.EXPECT().Name().Return("Buffer").AnyTimes()

		bufferAnalyzer = MakeBufferAnalyzerBuilder().
			WithPerfLogger(logger).
			WithTimeTeller(timeTeller).
			WithPeriod(1).
			WithBuffer(buffer).
			Build()
	})

	AfterEach(func() {
		mockCtrl.Finish()
	})

	It("should calculate average buffer level", func() {
		timeTeller.EXPECT().
			CurrentTime().
			Return(sim.VTimeInSec(0.1))
		buffer.EXPECT().Size().Return(1)

		bufferAnalyzer.Func(sim.HookCtx{
			Domain: buffer,
			Item:   gomock.Nil(),
			Pos:    sim.HookPosBufPush,
		})

		timeTeller.EXPECT().CurrentTime().Return(sim.VTimeInSec(1.1)).AnyTimes()
		buffer.EXPECT().Size().Return(2)
		logger.EXPECT().AddDataEntry(PerfAnalyzerEntry{
			Start:     0.0,
			End:       1.0,
			Where:     "Buffer",
			What:      "Level",
			EntryType: "Buffer",
			Value:     0.9,
			Unit:      "",
		})

		bufferAnalyzer.Func(sim.HookCtx{
			Domain: buffer,
			Item:   gomock.Nil(),
			Pos:    sim.HookPosBufPush,
		})
	})

	It("should report multiple periods together", func() {
		timeTeller.EXPECT().
			CurrentTime().
			Return(sim.VTimeInSec(0.1))
		buffer.EXPECT().Size().Return(1)

		bufferAnalyzer.Func(sim.HookCtx{
			Domain: buffer,
			Item:   gomock.Nil(),
			Pos:    sim.HookPosBufPush,
		})

		timeTeller.EXPECT().CurrentTime().Return(sim.VTimeInSec(2.1)).AnyTimes()
		buffer.EXPECT().Size().Return(2)
		logger.EXPECT().AddDataEntry(PerfAnalyzerEntry{
			Start:     0.0,
			End:       1.0,
			Where:     "Buffer",
			What:      "Level",
			EntryType: "Buffer",
			Value:     0.9,
			Unit:      "",
		})

		logger.EXPECT().AddDataEntry(PerfAnalyzerEntry{
			Start:     1.0,
			End:       2.0,
			Where:     "Buffer",
			What:      "Level",
			EntryType: "Buffer",
			Value:     1,
			Unit:      "",
		})

		bufferAnalyzer.Func(sim.HookCtx{
			Domain: buffer,
			Item:   gomock.Nil(),
			Pos:    sim.HookPosBufPush,
		})
	})
})
