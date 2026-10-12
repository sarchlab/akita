package queueing

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func newPipeline(width, numStages int) *Pipeline[int] {
	p := MakePipeline[int](width, numStages)
	return &p
}

func newPostBuf(capacity int) *Buffer[int] {
	b := MakeBuffer[int](capacity)
	return &b
}

func TestPipelineCanAcceptEmpty(t *testing.T) {
	p := newPipeline(2, 3)
	assert.True(t, p.CanAccept())
}

func TestPipelineCanAcceptFull(t *testing.T) {
	p := newPipeline(2, 3)
	p.Accept(1)
	p.Accept(2)
	assert.False(t, p.CanAccept())
}

func TestPipelineAcceptAssignsLane(t *testing.T) {
	p := newPipeline(2, 3)
	p.Accept(10)
	p.Accept(20)

	assert.Equal(t, 2, len(p.stages))
	assert.Equal(t, 0, p.stages[0].Lane)
	assert.Equal(t, 1, p.stages[1].Lane)
	assert.Equal(t, 0, p.stages[0].Stage)
	assert.Equal(t, 0, p.stages[1].Stage)
}

func TestPipelineTickAdvancesToPostBuf(t *testing.T) {
	// Single lane, single stage pipeline.
	p := newPipeline(1, 1)
	postBuf := newPostBuf(10)

	p.Accept(100)

	moved := p.Tick(postBuf)
	assert.True(t, moved)
	assert.Equal(t, 0, len(p.stages))
	assert.Equal(t, 1, postBuf.Size())
	value0, present0 := postBuf.Peek()
	assert.True(t, present0)
	assert.Equal(t, 100, value0)
}

func TestPipelineTickMultiStage(t *testing.T) {
	// 1 lane, 3 stages. Item should take 3 ticks to reach output.
	p := newPipeline(1, 3)
	postBuf := newPostBuf(10)

	p.Accept(42)

	// Tick 1: advance to stage 1.
	moved := p.Tick(postBuf)
	assert.True(t, moved)
	assert.Equal(t, 1, len(p.stages))
	assert.Equal(t, 1, p.stages[0].Stage)

	// Tick 2: advance to stage 2 (last stage).
	moved = p.Tick(postBuf)
	assert.True(t, moved)
	assert.Equal(t, 1, len(p.stages))
	assert.Equal(t, 2, p.stages[0].Stage)

	// Tick 3: output to postBuf.
	moved = p.Tick(postBuf)
	assert.True(t, moved)
	assert.Equal(t, 0, len(p.stages))
	assert.Equal(t, 1, postBuf.Size())
}

func TestPipelineTickPostBufFull(t *testing.T) {
	p := newPipeline(1, 1)
	postBuf := newPostBuf(0)

	p.Accept(10)

	moved := p.Tick(postBuf)
	assert.False(t, moved)
	assert.Equal(t, 1, len(p.stages)) // Item stays.
}

func TestPipelineTickNoMovement(t *testing.T) {
	p := newPipeline(1, 3)
	postBuf := newPostBuf(10)

	moved := p.Tick(postBuf)
	assert.False(t, moved) // Empty pipeline, nothing to move.
}

func TestPipelineMultiLane(t *testing.T) {
	p := newPipeline(2, 2)
	postBuf := newPostBuf(10)

	p.Accept(1)
	p.Accept(2)

	// Both items at stage 0.
	// Tick 1: both advance to stage 1 (last stage).
	p.Tick(postBuf)
	for _, s := range p.stages {
		assert.Equal(t, 1, s.Stage)
	}

	// Tick 2: both output.
	p.Tick(postBuf)
	assert.Equal(t, 0, len(p.stages))
	assert.Equal(t, 2, postBuf.Size())
}

func TestPipelineBlockedByNextStage(t *testing.T) {
	// 1 lane, 2 stages. Put items at both stages.
	p := newPipeline(1, 2)
	p.stages = []PipelineStage[int]{
		{Lane: 0, Stage: 0, Item: 10},
		{Lane: 0, Stage: 1, Item: 20},
	}

	// postBuf is full, so stage-1 item can't leave, stage-0 can't advance.
	postBuf := newPostBuf(0)
	moved := p.Tick(postBuf)
	assert.False(t, moved)
	assert.Equal(t, 2, len(p.stages))
}

func TestPipelineBlockedThenUnblocked(t *testing.T) {
	p := newPipeline(1, 2)
	p.stages = []PipelineStage[int]{
		{Lane: 0, Stage: 0, Item: 10},
		{Lane: 0, Stage: 1, Item: 20},
	}

	postBuf := newPostBuf(10)

	// Tick: stage-1 item outputs, stage-0 item advances.
	moved := p.Tick(postBuf)
	assert.True(t, moved)
	assert.Equal(t, 1, len(p.stages))
	assert.Equal(t, 1, p.stages[0].Stage)
	assert.Equal(t, 10, p.stages[0].Item)
	assert.Equal(t, 1, postBuf.Size())
	value1, present1 := postBuf.Peek()
	assert.True(t, present1)
	assert.Equal(t, 20, value1)
}

func TestPipelineStreamOfItems(t *testing.T) {
	p := newPipeline(1, 2)
	postBuf := newPostBuf(10)

	p.Accept(1)

	// Tick 1: item 1 advances to stage 1.
	p.Tick(postBuf)
	assert.Equal(t, 1, p.stages[0].Stage)

	p.Accept(2)

	// Tick 2: Item 1 outputs. Item 2 advances to stage 1.
	p.Tick(postBuf)
	assert.Equal(t, 1, postBuf.Size())
	assert.Equal(t, 1, len(p.stages))

	// Tick 3: Item 2 outputs.
	p.Tick(postBuf)
	assert.Equal(t, 2, postBuf.Size())

	value2, present2 := postBuf.Pop()
	assert.True(t, present2)
	assert.Equal(t, 1, value2)
	value3, present3 := postBuf.Pop()
	assert.True(t, present3)
	assert.Equal(t, 2, value3)
}

func TestPipelineFullAcceptDoesNotChangeContents(t *testing.T) {
	p := newPipeline(2, 2)
	p.Accept(0)
	p.Accept(1)
	before := p.Stages()
	assert.Panics(t, func() { p.Accept(2) })
	assert.Equal(t, before, p.Stages())
	sink := newPostBuf(2)
	p.Tick(sink)
	p.Tick(sink)
	assert.ElementsMatch(t, []int{0, 1}, sink.Elements())
}
