package pipelining

import (
	"github.com/sarchlab/akita/v3/sim"

	"github.com/golang/mock/gomock"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

type pipelineItem struct {
	taskID string
}

func (p pipelineItem) TaskID() string {
	return p.taskID
}

var _ = Describe("Pipeline", func() {
	var (
		mockCtrl           *gomock.Controller
		postPipelineBuffer *MockBuffer
		pipeline           Pipeline
	)

	BeforeEach(func() {
		mockCtrl = gomock.NewController(GinkgoT())
		postPipelineBuffer = NewMockBuffer(mockCtrl)
		pipeline = MakeBuilder().
			WithPipelineWidth(1).
			WithNumStage(100).
			WithCyclePerStage(2).
			WithPostPipelineBuffer(postPipelineBuffer).
			Build("Pipeline")
	})

	AfterEach(func() {
		mockCtrl.Finish()
	})

	It("should process items in pipeline", func() {
		item1 := pipelineItem{taskID: "1"}
		item2 := pipelineItem{taskID: "2"}

		canAccept1 := pipeline.CanAccept()
		Expect(canAccept1).To(BeTrue())

		pipeline.Accept(0, item1)
		canAccept2 := pipeline.CanAccept()
		Expect(canAccept2).To(BeFalse())

		madeProgress1 := pipeline.Tick(0)
		Expect(madeProgress1).To(BeTrue())

		canAccept3 := pipeline.CanAccept()
		Expect(canAccept3).To(BeFalse())

		madeProgress2 := pipeline.Tick(1)
		Expect(madeProgress2).To(BeTrue())

		canAccept4 := pipeline.CanAccept()
		Expect(canAccept4).To(BeTrue())
		pipeline.Accept(2, item2)

		for i := 2; i < 199; i++ {
			madeProgress := pipeline.Tick(sim.VTimeInSec(i))
			Expect(madeProgress).To(BeTrue())
		}

		postPipelineBuffer.EXPECT().CanPush().Return(true)
		postPipelineBuffer.EXPECT().Push(item1)

		madeProgress3 := pipeline.Tick(200)
		Expect(madeProgress3).To(BeTrue())

		madeProgress4 := pipeline.Tick(201)
		Expect(madeProgress4).To(BeTrue())

		postPipelineBuffer.EXPECT().CanPush().Return(false)
		madeProgress5 := pipeline.Tick(202)
		Expect(madeProgress5).To(BeFalse())

		postPipelineBuffer.EXPECT().CanPush().Return(true)
		postPipelineBuffer.EXPECT().Push(item2)
		madeProgress6 := pipeline.Tick(203)
		Expect(madeProgress6).To(BeTrue())

		madeProgress7 := pipeline.Tick(204)
		Expect(madeProgress7).To(BeFalse())
	})
})

var _ = Describe("Zero-Stage Pipeline", func() {
	var (
		mockCtrl           *gomock.Controller
		postPipelineBuffer *MockBuffer
		pipeline           Pipeline
	)

	BeforeEach(func() {
		mockCtrl = gomock.NewController(GinkgoT())
		postPipelineBuffer = NewMockBuffer(mockCtrl)
		pipeline = MakeBuilder().
			WithPipelineWidth(1).
			WithNumStage(0).
			WithCyclePerStage(2).
			WithPostPipelineBuffer(postPipelineBuffer).
			Build("Pipeline")
	})

	AfterEach(func() {
		mockCtrl.Finish()
	})

	It("should not accept if post buffer is full", func() {
		postPipelineBuffer.EXPECT().CanPush().Return(false)

		canAccept := pipeline.CanAccept()

		Expect(canAccept).To(BeFalse())
	})

	It("should forward to post buffer directory", func() {
		item1 := pipelineItem{taskID: "1"}

		postPipelineBuffer.EXPECT().CanPush().Return(true)
		postPipelineBuffer.EXPECT().Push(item1)

		canAccept := pipeline.CanAccept()
		pipeline.Accept(1, item1)

		Expect(canAccept).To(BeTrue())
	})
})
