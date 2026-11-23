package writeback

import (
	"github.com/sarchlab/akita/v5/mem"
	"github.com/sarchlab/akita/v5/sim/messaging"
	"github.com/sarchlab/akita/v5/sim/timing"
)

// pipelineMW runs the writeback cache's data pipeline. It holds only
// references: the component, the storage, and the pipeline stages. All mutable state is in comp.State.
type pipelineMW struct {
	comp *Comp

	storage *mem.Storage

	topParser   *topParser
	writeBuffer *writeBufferStage
	dirStage    *directoryStage
	bankStages  []*bankStage
	mshrStage   *mshrStage
}

// stage is one stage of the data pipeline. Tick advances it by one step and
// reports whether it made progress.
type stage interface {
	Tick() bool
}

// createInternalStages creates the pipeline stages, one bank stage per bank.
func (m *pipelineMW) createInternalStages() {
	spec := m.comp.Spec

	m.topParser = &topParser{cache: m}
	m.dirStage = &directoryStage{cache: m}

	numBanks := spec.NumBanks
	m.bankStages = make([]*bankStage, numBanks)
	for i := 0; i < numBanks; i++ {
		m.bankStages[i] = &bankStage{
			cache:         m,
			bankID:        i,
			pipelineWidth: spec.BankWidth,
		}
	}

	m.mshrStage = &mshrStage{cache: m}
	m.writeBuffer = &writeBufferStage{cache: m}
}

// Spec returns the immutable specification.
func (m *pipelineMW) Spec() Spec {
	return m.comp.Spec
}

// topPort returns the Top port.
func (m *pipelineMW) topPort() messaging.Port {
	return m.comp.Ports.Top
}

// bottomPort returns the Bottom port.
func (m *pipelineMW) bottomPort() messaging.Port {
	return m.comp.Ports.Bottom
}

// findPort resolves an address to the lower-memory port that serves it.
func (m *pipelineMW) findPort(address uint64) messaging.RemotePort {
	return m.comp.Spec.Bottom.Find(address)
}

// Handle runs one cycle of the cache pipeline, unless the cache is paused. When
// the cycle makes no progress, the cache sleeps until a pipeline item is due.
func (m *pipelineMW) Handle(_ timing.Event) bool {
	next := &m.comp.State
	cycle := m.currentCycle()

	if cacheState(next.CacheState) == cacheStatePaused {
		m.freezePipelines(cycle)
		return false
	}
	m.resumePipelines(cycle)

	madeProgress := m.runPipeline()
	if !madeProgress {
		m.scheduleWake()
	}

	return madeProgress
}

// currentCycle returns the clock cycle of the current simulation time.
func (m *pipelineMW) currentCycle() uint64 {
	return m.comp.Spec.Freq.Cycle(m.comp.CurrentTime())
}

// freezePipelines stops pipeline time at the end of the cycle before cycle: the
// tick that observes the pause does no pipeline work.
func (m *pipelineMW) freezePipelines(cycle uint64) {
	next := &m.comp.State
	if next.PipelinesFrozen {
		return
	}
	next.PipelinesFrozen = true

	for i := range next.BankPipelines {
		if p := &next.BankPipelines[i]; cycle > p.Cycle() {
			p.AdvanceTo(cycle - 1)
		}
	}
	if step := cycle * uint64(m.comp.Spec.NumReqPerCycle); step > next.DirPipeline.Cycle() {
		next.DirPipeline.AdvanceTo(step)
	}
}

// resumePipelines discards the paused cycles, so this tick is the first cycle
// of pipeline work after the freeze.
func (m *pipelineMW) resumePipelines(cycle uint64) {
	next := &m.comp.State
	if !next.PipelinesFrozen {
		return
	}
	next.PipelinesFrozen = false

	for i := range next.BankPipelines {
		if p := &next.BankPipelines[i]; cycle > p.Cycle() {
			p.SkipTo(cycle - 1)
		}
	}
	if step := cycle * uint64(m.comp.Spec.NumReqPerCycle); step > next.DirPipeline.Cycle() {
		next.DirPipeline.SkipTo(step)
	}
}

// scheduleWake asks for a tick at the next cycle a pipeline item becomes ready.
// Work blocked on anything else resumes through a port notification or through
// the progress of the stage that unblocks it.
func (m *pipelineMW) scheduleWake() {
	next := &m.comp.State
	spec := m.comp.Spec
	wake, found := uint64(0), false
	consider := func(cycle uint64) {
		if !found || cycle < wake {
			wake, found = cycle, true
		}
	}

	for i := range next.BankPipelines {
		if due, ok := next.BankPipelines[i].NextReadyCycle(); ok {
			consider(due)
		}
	}
	if due, ok := next.DirPipeline.NextReadyCycle(); ok {
		consider((due - 1) / uint64(spec.NumReqPerCycle))
	}

	if found {
		m.comp.TickAt(timing.VTimeInPicoSec(wake) * spec.Freq.Period())
	}
}

func (m *pipelineMW) runPipeline() bool {
	madeProgress := false

	spec := m.comp.Spec

	madeProgress = m.runStage(m.mshrStage, spec.NumReqPerCycle) || madeProgress

	for _, bs := range m.bankStages {
		madeProgress = bs.Tick() || madeProgress
	}

	madeProgress = m.runStage(m.writeBuffer, spec.NumReqPerCycle) || madeProgress
	madeProgress = m.runStage(m.dirStage, spec.NumReqPerCycle) || madeProgress
	madeProgress = m.runStage(m.topParser, spec.NumReqPerCycle) || madeProgress

	return madeProgress
}

func (m *pipelineMW) runStage(s stage, n int) bool {
	madeProgress := false
	for range n {
		madeProgress = s.Tick() || madeProgress
	}

	return madeProgress
}
