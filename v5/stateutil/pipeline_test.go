package stateutil

import (
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func newPipeline(width, numStages int) *Pipeline[int] {
	return &Pipeline[int]{
		Width:     width,
		NumStages: numStages,
	}
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

	assert.Equal(t, 2, len(p.Stages))
	assert.Equal(t, 0, p.Stages[0].Lane)
	assert.Equal(t, 1, p.Stages[1].Lane)
	assert.Equal(t, 0, p.Stages[0].Stage)
	assert.Equal(t, 0, p.Stages[1].Stage)
}

func TestPipelineAcceptCycleLeft(t *testing.T) {
	p := newPipeline(1, 4)
	p.Accept(42)
	assert.Equal(t, 0, p.Stages[0].CycleLeft) // CycleLeft starts at 0
}

func TestPipelineTickAdvancesToPostBuf(t *testing.T) {
	// Single lane, single stage pipeline.
	p := newPipeline(1, 1)
	postBuf := &Buffer[int]{BufferName: "post", Cap: 10}

	p.Accept(100)

	// CycleLeft should be 0 for a 1-stage pipeline.
	assert.Equal(t, 0, p.Stages[0].CycleLeft)

	moved := p.Tick(postBuf)
	assert.True(t, moved)
	assert.Equal(t, 0, len(p.Stages))
	assert.Equal(t, 1, postBuf.Size())

	v, ok := postBuf.PopTyped()
	assert.True(t, ok)
	assert.Equal(t, 100, v)
}

func TestPipelineTickMultiStage(t *testing.T) {
	// 1 lane, 3 stages. Item should take 3 ticks to reach output.
	// With CycleLeft=0, each tick advances one stage.
	p := newPipeline(1, 3)
	postBuf := &Buffer[int]{BufferName: "post", Cap: 10}

	p.Accept(42)
	// Stage=0, CycleLeft=0

	// Tick 1: advance to stage 1.
	moved := p.Tick(postBuf)
	assert.True(t, moved)
	assert.Equal(t, 1, len(p.Stages))
	assert.Equal(t, 1, p.Stages[0].Stage)
	assert.Equal(t, 0, p.Stages[0].CycleLeft)

	// Tick 2: advance to stage 2 (last stage).
	moved = p.Tick(postBuf)
	assert.True(t, moved)
	assert.Equal(t, 1, len(p.Stages))
	assert.Equal(t, 2, p.Stages[0].Stage)

	// Tick 3: output to postBuf.
	moved = p.Tick(postBuf)
	assert.True(t, moved)
	assert.Equal(t, 0, len(p.Stages))
	assert.Equal(t, 1, postBuf.Size())
}

func TestPipelineTickPostBufFull(t *testing.T) {
	p := newPipeline(1, 1)
	postBuf := &Buffer[int]{BufferName: "post", Cap: 0}

	p.Accept(10)

	moved := p.Tick(postBuf)
	assert.False(t, moved)
	assert.Equal(t, 1, len(p.Stages)) // Item stays.
}

func TestPipelineTickNoMovement(t *testing.T) {
	p := newPipeline(1, 3)
	postBuf := &Buffer[int]{BufferName: "post", Cap: 10}

	moved := p.Tick(postBuf)
	assert.False(t, moved) // Empty pipeline, nothing to move.
}

func TestPipelineTickFunc(t *testing.T) {
	p := newPipeline(1, 1)
	p.Accept(55)

	var received []int
	moved := p.TickFunc(func(item int) bool {
		received = append(received, item)
		return true
	})

	assert.True(t, moved)
	assert.Equal(t, []int{55}, received)
	assert.Equal(t, 0, len(p.Stages))
}

func TestPipelineTickFuncReject(t *testing.T) {
	p := newPipeline(1, 1)
	p.Accept(55)

	moved := p.TickFunc(func(item int) bool {
		return false
	})

	assert.False(t, moved)
	assert.Equal(t, 1, len(p.Stages))
}

func TestPipelineMultiLane(t *testing.T) {
	p := newPipeline(2, 2)
	postBuf := &Buffer[int]{BufferName: "post", Cap: 10}

	p.Accept(1)
	p.Accept(2)

	// Both items at stage 0 with CycleLeft=0.
	// Tick 1: both advance to stage 1 (last stage).
	p.Tick(postBuf)
	for _, s := range p.Stages {
		assert.Equal(t, 1, s.Stage)
	}

	// Tick 2: both output.
	p.Tick(postBuf)
	assert.Equal(t, 0, len(p.Stages))
	assert.Equal(t, 2, postBuf.Size())
}

func TestPipelineBlockedByNextStage(t *testing.T) {
	// 1 lane, 2 stages. Put items at both stages.
	p := &Pipeline[int]{
		Width:     1,
		NumStages: 2,
		Stages: []PipelineStage[int]{
			{Lane: 0, Stage: 0, CycleLeft: 0},
			{Lane: 0, Stage: 1, CycleLeft: 0},
		},
	}
	p.Stages[0].Item = 10
	p.Stages[1].Item = 20

	// postBuf is full, so stage-1 item can't leave, stage-0 can't advance.
	postBuf := &Buffer[int]{BufferName: "post", Cap: 0}
	moved := p.Tick(postBuf)
	assert.False(t, moved)
	assert.Equal(t, 2, len(p.Stages))
}

func TestPipelineBlockedThenUnblocked(t *testing.T) {
	p := &Pipeline[int]{
		Width:     1,
		NumStages: 2,
		Stages: []PipelineStage[int]{
			{Lane: 0, Stage: 0, Item: 10, CycleLeft: 0},
			{Lane: 0, Stage: 1, Item: 20, CycleLeft: 0},
		},
	}

	postBuf := &Buffer[int]{BufferName: "post", Cap: 10}

	// Tick: stage-1 item outputs, stage-0 item advances.
	moved := p.Tick(postBuf)
	assert.True(t, moved)
	assert.Equal(t, 1, len(p.Stages))
	assert.Equal(t, 1, p.Stages[0].Stage)
	assert.Equal(t, 10, p.Stages[0].Item)
	assert.Equal(t, 1, postBuf.Size())

	v, _ := postBuf.PopTyped()
	assert.Equal(t, 20, v)
}

func TestPipelineJSONRoundTrip(t *testing.T) {
	p := &Pipeline[string]{
		Width:     2,
		NumStages: 3,
		Stages: []PipelineStage[string]{
			{Lane: 0, Stage: 1, Item: "alpha", CycleLeft: 1},
			{Lane: 1, Stage: 0, Item: "beta", CycleLeft: 2},
		},
	}

	data, err := json.Marshal(p)
	require.NoError(t, err)

	var p2 Pipeline[string]
	err = json.Unmarshal(data, &p2)
	require.NoError(t, err)

	assert.Equal(t, 2, p2.Width)
	assert.Equal(t, 3, p2.NumStages)
	assert.Equal(t, 2, len(p2.Stages))
	assert.Equal(t, "alpha", p2.Stages[0].Item)
	assert.Equal(t, "beta", p2.Stages[1].Item)
	assert.Equal(t, 1, p2.Stages[0].CycleLeft)
	assert.Equal(t, 2, p2.Stages[1].CycleLeft)
}

func TestPipelineJSONRoundTripStruct(t *testing.T) {
	type myItem struct {
		ID int `json:"id"`
	}

	p := &Pipeline[myItem]{
		Width:     1,
		NumStages: 2,
		Stages: []PipelineStage[myItem]{
			{Lane: 0, Stage: 0, Item: myItem{ID: 7}, CycleLeft: 1},
		},
	}

	data, err := json.Marshal(p)
	require.NoError(t, err)

	var p2 Pipeline[myItem]
	err = json.Unmarshal(data, &p2)
	require.NoError(t, err)

	assert.Equal(t, 7, p2.Stages[0].Item.ID)
}

func TestPipelineStreamOfItems(t *testing.T) {
	// Verify a stream of items flows through correctly.
	// 1 lane, 2 stages. With CycleLeft=0, each item takes 2 ticks.
	p := newPipeline(1, 2)
	postBuf := &Buffer[int]{BufferName: "post", Cap: 10}

	// Accept item 1.
	p.Accept(1)

	// Tick 1: item 1 advances to stage 1.
	p.Tick(postBuf)
	assert.Equal(t, 1, p.Stages[0].Stage)

	// Accept item 2 into stage 0.
	p.Accept(2)

	// Tick 2: Item 1 outputs. Item 2 advances to stage 1.
	p.Tick(postBuf)
	assert.Equal(t, 1, postBuf.Size())
	assert.Equal(t, 1, len(p.Stages))

	// Tick 3: Item 2 outputs.
	p.Tick(postBuf)
	assert.Equal(t, 2, postBuf.Size())

	v1, _ := postBuf.PopTyped()
	v2, _ := postBuf.PopTyped()
	assert.Equal(t, 1, v1)
	assert.Equal(t, 2, v2)
}

func TestPipelineCycleLeftDecrement(t *testing.T) {
	// Test that CycleLeft > 0 prevents advancement.
	p := &Pipeline[int]{
		Width:     1,
		NumStages: 2,
		Stages: []PipelineStage[int]{
			{Lane: 0, Stage: 0, Item: 99, CycleLeft: 2},
		},
	}
	postBuf := &Buffer[int]{BufferName: "post", Cap: 10}

	// Tick 1: CycleLeft 2→1.
	moved := p.Tick(postBuf)
	assert.True(t, moved)
	assert.Equal(t, 1, p.Stages[0].CycleLeft)
	assert.Equal(t, 0, p.Stages[0].Stage)

	// Tick 2: CycleLeft 1→0.
	moved = p.Tick(postBuf)
	assert.True(t, moved)
	assert.Equal(t, 0, p.Stages[0].CycleLeft)
	assert.Equal(t, 0, p.Stages[0].Stage)

	// Tick 3: advance to stage 1.
	moved = p.Tick(postBuf)
	assert.True(t, moved)
	assert.Equal(t, 1, p.Stages[0].Stage)

	// Tick 4: output.
	moved = p.Tick(postBuf)
	assert.True(t, moved)
	assert.Equal(t, 0, len(p.Stages))
	assert.Equal(t, 1, postBuf.Size())
}
