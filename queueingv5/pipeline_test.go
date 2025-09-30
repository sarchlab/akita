package queueingv5

import (
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

type testPipelineItem struct {
	taskID string
}

func (p testPipelineItem) TaskID() string {
	return p.taskID
}

var _ = Describe("Pipeline", func() {
	var (
		postPipelineBuffer *Buffer
		pipeline           *Pipeline
	)

	BeforeEach(func() {
		postPipelineBuffer = NewBuffer("PostPipelineBuffer", 10)
		pipeline = NewPipelineBuilder().
			WithPipelineWidth(1).
			WithNumStage(3).
			WithCyclePerStage(2).
			WithPostPipelineBuffer(postPipelineBuffer).
			Build("TestPipeline")
	})

	Context("when newly created", func() {
		It("should have correct name", func() {
			Expect(pipeline.Name()).To(Equal("TestPipeline"))
		})

		It("should be able to accept items", func() {
			Expect(pipeline.CanAccept()).To(BeTrue())
		})
	})

	Context("when processing items", func() {
		It("should process items through pipeline stages", func() {
			item1 := testPipelineItem{taskID: "1"}
			item2 := testPipelineItem{taskID: "2"}

			// Accept first item
			Expect(pipeline.CanAccept()).To(BeTrue())
			pipeline.Accept(item1)
			Expect(pipeline.CanAccept()).To(BeFalse())

			// First tick - item1 moves forward
			madeProgress1 := pipeline.Tick()
			Expect(madeProgress1).To(BeTrue())

			// Second tick - item1 moves forward, can accept new item
			madeProgress2 := pipeline.Tick()
			Expect(madeProgress2).To(BeTrue())
			Expect(pipeline.CanAccept()).To(BeTrue())

			// Accept second item
			pipeline.Accept(item2)

			// Continue ticking until item1 reaches post-pipeline buffer
			for i := 0; i < 4; i++ {
				pipeline.Tick()
			}

			// Check that item1 is in post-pipeline buffer
			Expect(postPipelineBuffer.Size()).To(Equal(1))
			Expect(postPipelineBuffer.Peek()).To(Equal(item1))
		})
	})

	Context("with zero stages", func() {
		BeforeEach(func() {
			pipeline = NewPipelineBuilder().
				WithPipelineWidth(1).
				WithNumStage(0).
				WithCyclePerStage(2).
				WithPostPipelineBuffer(postPipelineBuffer).
				Build("ZeroStagePipeline")
		})

		It("should directly push to post-pipeline buffer", func() {
			item := testPipelineItem{taskID: "1"}

			Expect(pipeline.CanAccept()).To(BeTrue())
			pipeline.Accept(item)

			Expect(postPipelineBuffer.Size()).To(Equal(1))
			Expect(postPipelineBuffer.Peek()).To(Equal(item))
		})
	})

	Context("when post-pipeline buffer is full", func() {
		BeforeEach(func() {
			// Fill post-pipeline buffer
			for i := 0; i < 10; i++ {
				postPipelineBuffer.Push("dummy")
			}
		})

		It("should block when trying to move to full buffer", func() {
			item := testPipelineItem{taskID: "1"}
			pipeline.Accept(item)

			// Tick through all stages until item reaches the end
			for i := 0; i < 6; i++ {
				pipeline.Tick()
			}

			// Item should still be in pipeline since buffer is full
			Expect(postPipelineBuffer.Size()).To(Equal(10))

			// Clear one space and tick again
			postPipelineBuffer.Pop()
			madeProgress := pipeline.Tick()
			Expect(madeProgress).To(BeTrue())

			// Now item should be in post-pipeline buffer
			Expect(postPipelineBuffer.Size()).To(Equal(10))
		})
	})

	Context("when cleared", func() {
		It("should remove all items from pipeline", func() {
			item := testPipelineItem{taskID: "1"}
			pipeline.Accept(item)
			pipeline.Tick()

			pipeline.Clear()

			Expect(pipeline.CanAccept()).To(BeTrue())
			// Ticking should not make progress since pipeline is empty
			madeProgress := pipeline.Tick()
			Expect(madeProgress).To(BeFalse())
		})
	})
})