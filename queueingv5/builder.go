package queueingv5

import "github.com/sarchlab/akita/v4/sim"

// PipelineBuilder can build pipelines.
type PipelineBuilder struct {
	width           int
	numStage        int
	cyclePerStage   int
	postPipelineBuf *Buffer
}

// NewPipelineBuilder creates a default pipeline builder.
func NewPipelineBuilder() *PipelineBuilder {
	return &PipelineBuilder{
		width:         1,
		numStage:      5,
		cyclePerStage: 1,
	}
}

// WithPipelineWidth sets the number of lanes in the pipeline. If width=4,
// 4 elements can be in the same stage at the same time.
func (b *PipelineBuilder) WithPipelineWidth(n int) *PipelineBuilder {
	b.width = n
	return b
}

// WithNumStage sets the number of pipeline stages.
func (b *PipelineBuilder) WithNumStage(n int) *PipelineBuilder {
	b.numStage = n
	return b
}

// WithCyclePerStage sets the number of cycles that each element needs to
// stage in each stage.
func (b *PipelineBuilder) WithCyclePerStage(n int) *PipelineBuilder {
	b.cyclePerStage = n
	return b
}

// WithPostPipelineBuffer sets the buffer that the elements can be pushed to
// after passing through the pipeline.
func (b *PipelineBuilder) WithPostPipelineBuffer(buf *Buffer) *PipelineBuilder {
	b.postPipelineBuf = buf
	return b
}

// Build builds a pipeline.
func (b *PipelineBuilder) Build(name string) *Pipeline {
	sim.NameMustBeValid(name)

	p := &Pipeline{
		name:            name,
		width:           b.width,
		numStage:        b.numStage,
		cyclePerStage:   b.cyclePerStage,
		postPipelineBuf: b.postPipelineBuf,
	}

	p.Clear()

	return p
}