package pipelining

import "github.com/sarchlab/akita/v3/sim"

// A Builder can build pipelines.
type Builder struct {
	width           int
	numStage        int
	cyclePerStage   int
	postPipelineBuf sim.Buffer
}

// MakeBuilder creates a default builder
func MakeBuilder() Builder {
	return Builder{
		width:         1,
		numStage:      5,
		cyclePerStage: 1,
	}
}

// WithPipelineWidth sets the number of lanes in the pipeline. If width=4,
// 4 elements can be in the same stage at the same time.
func (b Builder) WithPipelineWidth(n int) Builder {
	b.width = n
	return b
}

// WithNumStage sets the number of pipeline stages
func (b Builder) WithNumStage(n int) Builder {
	b.numStage = n
	return b
}

// WithCyclePerStage sets the the number of cycles that each element needs to
// stage in each stage.
func (b Builder) WithCyclePerStage(n int) Builder {
	b.cyclePerStage = n
	return b
}

// WithPostPipelineBuffer sets the buffer that the elements can be pushed to
// after passing through the pipeline.
func (b Builder) WithPostPipelineBuffer(buf sim.Buffer) Builder {
	b.postPipelineBuf = buf
	return b
}

// Build builds a pipeline.
func (b Builder) Build(name string) Pipeline {
	sim.NameMustBeValid(name)

	p := &pipelineImpl{
		name:            name,
		width:           b.width,
		numStage:        b.numStage,
		cyclePerStage:   b.cyclePerStage,
		postPipelineBuf: b.postPipelineBuf,
	}

	p.Clear()

	return p
}
