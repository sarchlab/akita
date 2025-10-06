package queueingv5

// This example demonstrates basic usage of the queueingv5 package.

// Define a pipeline item type that implements PipelineItem interface
type myItem struct{ id string }

func (m myItem) TaskID() string { return m.id }

func ExampleBuffer() {
	// Create a buffer with capacity 3
	buffer := NewBuffer("MyBuffer", 3)

	// Add some items
	buffer.Push("item1")
	buffer.Push("item2")

	// Check capacity and size
	_ = buffer.Capacity() // returns 3
	_ = buffer.Size()     // returns 2

	// Pop items in FIFO order
	item1 := buffer.Pop() // returns "item1"
	item2 := buffer.Pop() // returns "item2"

	_ = item1
	_ = item2
}

func ExamplePipeline() {
	// Create a post-pipeline buffer
	postBuffer := NewBuffer("PostBuffer", 10)

	// Create a pipeline using the builder pattern
	pipeline := NewPipelineBuilder().
		WithPipelineWidth(1).
		WithNumStage(3).
		WithCyclePerStage(2).
		WithPostPipelineBuffer(postBuffer).
		Build("MyPipeline")

	// Create an item
	item := myItem{id: "task1"}

	// Add item to pipeline
	if pipeline.CanAccept() {
		pipeline.Accept(item)
	}

	// Tick the pipeline to process items
	for pipeline.Tick() {
		// Continue until no more progress is made
	}

	// Items will eventually appear in the post-pipeline buffer
	_ = postBuffer.Size() // Check how many items completed
}