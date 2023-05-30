// Package pipelining provides a pipeline definition.
package pipelining

import (
	"reflect"

	"github.com/sarchlab/akita/v3/sim"
	"github.com/sarchlab/akita/v3/tracing"
)

// PipelineItem is an item that can pass through a pipeline.
type PipelineItem interface {
	TaskID() string
}

// Pipeline allows simulation designers to define pipeline structures.
type Pipeline interface {
	tracing.NamedHookable

	// Tick moves elements in the pipeline forward.
	Tick(now sim.VTimeInSec) (madeProgress bool)

	// CanAccept checks if the pipeline can accept a new element.
	CanAccept() bool

	// Accept adds an element to the pipeline. If the first pipeline stage is
	// currently occupied, this function panics.
	Accept(now sim.VTimeInSec, elem PipelineItem)

	// Clear discards all the items that are currently in the pipeline.
	Clear()
}

// NewPipeline creates a default pipeline
//
// Deprecated: use PipelineBuilder instead.
func NewPipeline(
	name string,
	numStage, cyclePerStage int,
	postPipelineBuf sim.Buffer,
) Pipeline {
	sim.NameMustBeValid(name)

	p := &pipelineImpl{
		width:           1,
		numStage:        numStage,
		cyclePerStage:   cyclePerStage,
		postPipelineBuf: postPipelineBuf,
	}

	p.Clear()

	return p
}

type pipelineStageInfo struct {
	elem      PipelineItem
	cycleLeft int
}

type pipelineImpl struct {
	sim.HookableBase
	name            string
	width           int
	numStage        int
	cyclePerStage   int
	postPipelineBuf sim.Buffer
	stages          [][]pipelineStageInfo
}

func (p *pipelineImpl) Name() string {
	return p.name
}

// Clear discards all the items in the pipeline.
func (p *pipelineImpl) Clear() {
	p.stages = make([][]pipelineStageInfo, p.width)
	for i := 0; i < p.width; i++ {
		p.stages[i] = make([]pipelineStageInfo, p.numStage)
	}
}

// Tick moves elements in the pipeline forward.
func (p *pipelineImpl) Tick(now sim.VTimeInSec) (madeProgress bool) {
	for lane := 0; lane < p.width; lane++ {
		for i := p.numStage - 1; i >= 0; i-- {
			stage := &p.stages[lane][i]

			if stage.elem == nil {
				continue
			}

			if stage.cycleLeft > 0 {
				stage.cycleLeft--
				madeProgress = true
				continue
			}

			if i == p.numStage-1 {
				madeProgress =
					p.tryMoveToPostPipelineBuffer(now, stage) || madeProgress
			} else {
				madeProgress = p.tryMoveToNextStage(lane, i) || madeProgress
			}
		}
	}

	return madeProgress
}

func (p *pipelineImpl) tryMoveToPostPipelineBuffer(
	_ sim.VTimeInSec,
	stage *pipelineStageInfo,
) (succeed bool) {
	if !p.postPipelineBuf.CanPush() {
		return false
	}

	tracing.EndTask(stage.elem.TaskID()+"pipeline", p)

	p.postPipelineBuf.Push(stage.elem)
	stage.elem = nil

	return true
}

func (p *pipelineImpl) tryMoveToNextStage(
	lane int,
	stageNum int,
) (succeed bool) {
	stage := &p.stages[lane][stageNum]
	nextStage := &p.stages[lane][stageNum+1]
	if nextStage.elem != nil {
		return false
	}

	nextStage.elem = stage.elem
	nextStage.cycleLeft = p.cyclePerStage - 1
	stage.elem = nil
	return true
}

// CanAccept checks if the pipeline can accept a new element.
func (p *pipelineImpl) CanAccept() bool {
	if p.numStage == 0 {
		return p.postPipelineBuf.CanPush()
	}

	for lane := 0; lane < p.width; lane++ {
		if p.stages[lane][0].elem == nil {
			return true
		}
	}

	return false
}

// Accept adds an element to the pipeline. If the first pipeline stage is
// currently occupied, this function panics.
func (p *pipelineImpl) Accept(_ sim.VTimeInSec, elem PipelineItem) {
	if p.numStage == 0 {
		p.postPipelineBuf.Push(elem)
		return
	}

	for lane := 0; lane < p.width; lane++ {
		if p.stages[lane][0].elem != nil {
			continue
		}

		p.stages[lane][0].elem = elem
		p.stages[lane][0].cycleLeft = p.cyclePerStage - 1

		tracing.StartTask(
			elem.TaskID()+"pipeline",
			elem.TaskID(),
			p,
			"pipeline",
			reflect.TypeOf(elem).String(),
			nil,
		)

		return
	}

	panic("pipeline is not free. Use can push before pushing.")
}
