package queueing

import (
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
		postPipelineBuffer *bufferImpl
		pipeline           Pipeline
	)

	BeforeEach(func() {
		mockCtrl = gomock.NewController(GinkgoT())
		postPipelineBuffer = NewBuffer("PostPipelineBuffer", 1)
		pipeline = MakePipelineBuilder().
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

		// Cycle 0, inject item1
		canAccept1 := pipeline.CanAccept()
		Expect(canAccept1).To(BeTrue())
		pipeline.Accept(item1)
		canAccept2 := pipeline.CanAccept()
		Expect(canAccept2).To(BeFalse())

		// Cycle 1, process item1
		madeProgress1 := pipeline.Tick()
		Expect(madeProgress1).To(BeTrue())
		canAccept3 := pipeline.CanAccept()
		Expect(canAccept3).To(BeFalse())

		// Cycle 3, process item1, inject item2
		madeProgress2 := pipeline.Tick()
		Expect(madeProgress2).To(BeTrue())
		canAccept4 := pipeline.CanAccept()
		Expect(canAccept4).To(BeTrue())
		pipeline.Accept(item2)

		// Cycle 4-198, process item1 and item2
		for i := 2; i < 199; i++ {
			madeProgress := pipeline.Tick()
			Expect(madeProgress).To(BeTrue())
			Expect(postPipelineBuffer.Size()).To(Equal(0))
		}

		// Cycle 199, pop item1
		madeProgress3 := pipeline.Tick()
		Expect(madeProgress3).To(BeTrue())
		Expect(postPipelineBuffer.Size()).To(Equal(1))
		Expect(postPipelineBuffer.Peek()).To(Equal(item1))

		// Cycle 200, process item2
		madeProgress4 := pipeline.Tick()
		Expect(madeProgress4).To(BeTrue())
		Expect(postPipelineBuffer.Size()).To(Equal(1))
		Expect(postPipelineBuffer.Peek()).To(Equal(item1))

		// Cycle 201, pop item 2 failed
		madeProgress5 := pipeline.Tick()
		Expect(madeProgress5).To(BeFalse())
		Expect(postPipelineBuffer.Size()).To(Equal(1))
		Expect(postPipelineBuffer.Peek()).To(Equal(item1))

		// Cycle 202, remove item 1 and pop item 2
		postPipelineBuffer.Pop()
		madeProgress6 := pipeline.Tick()
		Expect(madeProgress6).To(BeTrue())
		Expect(postPipelineBuffer.Size()).To(Equal(1))
		Expect(postPipelineBuffer.Peek()).To(Equal(item2))
	})
})

var _ = Describe("Zero-Stage Pipeline", func() {
	var (
		mockCtrl           *gomock.Controller
		postPipelineBuffer *bufferImpl
		pipeline           Pipeline
	)

	BeforeEach(func() {
		mockCtrl = gomock.NewController(GinkgoT())
		postPipelineBuffer = NewBuffer("PostPipelineBuffer", 1)
		pipeline = MakePipelineBuilder().
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
		item1 := pipelineItem{taskID: "1"}
		postPipelineBuffer.Push(item1)

		canAccept := pipeline.CanAccept()

		Expect(canAccept).To(BeFalse())
	})

	It("should forward to post buffer directory", func() {
		item1 := pipelineItem{taskID: "1"}

		canAccept := pipeline.CanAccept()
		pipeline.Accept(item1)

		Expect(canAccept).To(BeTrue())
		Expect(postPipelineBuffer.Size()).To(Equal(1))
		Expect(postPipelineBuffer.Peek()).To(Equal(item1))
	})
})
