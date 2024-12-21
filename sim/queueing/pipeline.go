package queueing

import (
	"reflect"

	"github.com/sarchlab/akita/v4/sim/hooking"
	"github.com/sarchlab/akita/v4/sim/naming"
	"github.com/sarchlab/akita/v4/sim/serialization"
	"github.com/sarchlab/akita/v4/sim/simulation"
)

// PipelineItem is an item that can pass through a pipeline.
type PipelineItem interface {
	serialization.Serializable
	TaskID() string
}

// Pipeline allows simulation designers to define pipeline structures.
type Pipeline interface {
	naming.Named
	hooking.Hookable
	simulation.StateHolder

	// Tick moves elements in the pipeline forward.
	Tick() (madeProgress bool)

	// CanAccept checks if the pipeline can accept a new element.
	CanAccept() bool

	// Accept adds an element to the pipeline. If the first pipeline stage is
	// currently occupied, this function panics.
	Accept(elem PipelineItem)

	// Clear discards all the items that are currently in the pipeline.
	Clear()
}

func init() {
	serialization.RegisterType(reflect.TypeOf(&pipelineState{}))
}

type pipelineState struct {
	name   string
	stages [][]pipelineStageInfo
}

func (s *pipelineState) Name() string {
	return s.name
}

func (s *pipelineState) Serialize() (map[string]any, error) {
	stages := make([][]map[string]any, len(s.stages))

	for i := range s.stages {
		stages[i] = make([]map[string]any, len(s.stages[i]))
		for j := range s.stages[i] {
			stages[i][j] = map[string]any{
				"elem":      s.stages[i][j].elem,
				"cycleLeft": s.stages[i][j].cycleLeft,
			}
		}
	}

	return map[string]any{
		"stages": stages,
	}, nil
}

func (s *pipelineState) Deserialize(state map[string]any) error {
	pipesMap := state["stages"].([]any)
	s.stages = make([][]pipelineStageInfo, len(pipesMap))

	for i := range pipesMap {
		pipeMap := pipesMap[i].([]any)

		s.stages[i] = make([]pipelineStageInfo, len(pipeMap))

		for j := range pipeMap {
			stageMap := pipeMap[j].(map[string]any)
			s.stages[i][j] = pipelineStageInfo{
				cycleLeft: stageMap["cycleLeft"].(int),
			}

			if stageMap["elem"] != nil {
				s.stages[i][j].elem = stageMap["elem"].(PipelineItem)
			}
		}
	}

	return nil
}

type pipelineStageInfo struct {
	elem      PipelineItem
	cycleLeft int
}

type pipelineImpl struct {
	hooking.HookableBase
	*pipelineState

	width           int
	numStage        int
	cyclePerStage   int
	postPipelineBuf Buffer
}

func (p *pipelineImpl) Name() string {
	return p.name
}

func (p *pipelineImpl) State() simulation.State {
	return p.pipelineState
}

func (p *pipelineImpl) SetState(state simulation.State) {
	p.pipelineState = state.(*pipelineState)
}

// Clear discards all the items in the pipeline.
func (p *pipelineImpl) Clear() {
	p.stages = make([][]pipelineStageInfo, p.width)
	for i := 0; i < p.width; i++ {
		p.stages[i] = make([]pipelineStageInfo, p.numStage)
	}
}

// Tick moves elements in the pipeline forward.
func (p *pipelineImpl) Tick() (madeProgress bool) {
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
					p.tryMoveToPostPipelineBuffer(stage) || madeProgress
			} else {
				madeProgress = p.tryMoveToNextStage(lane, i) || madeProgress
			}
		}
	}

	return madeProgress
}

func (p *pipelineImpl) tryMoveToPostPipelineBuffer(
	stage *pipelineStageInfo,
) (succeed bool) {
	if !p.postPipelineBuf.CanPush() {
		return false
	}

	// tracing.EndTask(stage.elem.TaskID()+"pipeline", p)

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
func (p *pipelineImpl) Accept(elem PipelineItem) {
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

		// tracing.StartTask(
		// 	elem.TaskID()+"pipeline",
		// 	elem.TaskID(),
		// 	p,
		// 	"pipeline",
		// 	reflect.TypeOf(elem).String(),
		// 	nil,
		// )

		return
	}

	panic("pipeline is not free. Use can push before pushing.")
}

// A PipelineBuilder can build pipelines.
type PipelineBuilder struct {
	sim             simulation.Simulation
	width           int
	numStage        int
	cyclePerStage   int
	postPipelineBuf Buffer
}

// MakePipelineBuilder creates a default builder
func MakePipelineBuilder() PipelineBuilder {
	return PipelineBuilder{
		width:         1,
		numStage:      5,
		cyclePerStage: 1,
	}
}

// WithSimulation sets the simulation that the pipeline belongs to.
func (b PipelineBuilder) WithSimulation(
	sim simulation.Simulation,
) PipelineBuilder {
	b.sim = sim
	return b
}

// WithPipelineWidth sets the number of lanes in the pipeline. If width=4,
// 4 elements can be in the same stage at the same time.
func (b PipelineBuilder) WithPipelineWidth(n int) PipelineBuilder {
	b.width = n
	return b
}

// WithNumStage sets the number of pipeline stages
func (b PipelineBuilder) WithNumStage(n int) PipelineBuilder {
	b.numStage = n
	return b
}

// WithCyclePerStage sets the the number of cycles that each element needs to
// stage in each stage.
func (b PipelineBuilder) WithCyclePerStage(n int) PipelineBuilder {
	b.cyclePerStage = n
	return b
}

// WithPostPipelineBuffer sets the buffer that the elements can be pushed to
// after passing through the pipeline.
func (b PipelineBuilder) WithPostPipelineBuffer(buf Buffer) PipelineBuilder {
	b.postPipelineBuf = buf
	return b
}

// Build builds a pipeline.
func (b PipelineBuilder) Build(name string) Pipeline {
	naming.NameMustBeValid(name)

	p := &pipelineImpl{
		pipelineState: &pipelineState{
			name: name,
		},
		width:           b.width,
		numStage:        b.numStage,
		cyclePerStage:   b.cyclePerStage,
		postPipelineBuf: b.postPipelineBuf,
	}

	p.Clear()

	b.sim.RegisterStateHolder(p)

	return p
}
