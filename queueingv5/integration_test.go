package queueingv5

import (
	"fmt"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

// workItem is a test item that implements PipelineItem
type workItem struct {
	id   string
	data int
}

func (w workItem) TaskID() string {
	return w.id
}

var _ = Describe("Integration Test", func() {
	It("should demonstrate complete buffer and pipeline integration", func() {
		// Create a chain of buffers and pipeline
		inputBuffer := NewBuffer("InputBuffer", 5)
		postPipelineBuffer := NewBuffer("PostPipelineBuffer", 10)

		// Create a pipeline that processes items from input to output
		pipeline := NewPipelineBuilder().
			WithPipelineWidth(2).
			WithNumStage(3).
			WithCyclePerStage(1).
			WithPostPipelineBuffer(postPipelineBuffer).
			Build("ProcessingPipeline")

		// Add work items to input buffer
		for i := 0; i < 4; i++ {
			item := workItem{id: fmt.Sprintf("work-%d", i), data: i * 10}
			inputBuffer.Push(item)
		}

		Expect(inputBuffer.Size()).To(Equal(4))

		// Process items through the pipeline
		processedCount := 0
		for inputBuffer.Size() > 0 || !isEmptyPipeline(pipeline) || postPipelineBuffer.Size() > 0 {
			// Move items from input buffer to pipeline
			if inputBuffer.Size() > 0 && pipeline.CanAccept() {
				item := inputBuffer.Pop()
				pipeline.Accept(item.(workItem))
			}

			// Tick the pipeline
			pipeline.Tick()

			// Process completed items
			for postPipelineBuffer.Size() > 0 {
				completed := postPipelineBuffer.Pop()
				item := completed.(workItem)
				Expect(item.id).To(ContainSubstring("work-"))
				processedCount++
			}
		}

		// Verify all items were processed
		Expect(processedCount).To(Equal(4))
		Expect(inputBuffer.Size()).To(Equal(0))
		Expect(postPipelineBuffer.Size()).To(Equal(0))
	})
})

// Helper function to check if pipeline is empty
func isEmptyPipeline(p *Pipeline) bool {
	// Try to tick and see if any progress is made
	// If no progress is made, the pipeline is effectively empty
	return !p.Tick()
}