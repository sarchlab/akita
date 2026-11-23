package writethrough

import (
	"github.com/sarchlab/akita/v5/mem"
	"github.com/sarchlab/akita/v5/sim/messaging"
	"github.com/sarchlab/akita/v5/sim/timing"
)

// pipelineMW runs the cache data pipeline. It holds only references: the
// component, the storage from Resources, and the pipeline
// stages, which keep their mutable data in State.
type pipelineMW struct {
	comp *Comp

	storage *mem.Storage

	intakeStage      *intake
	directoryStage   *directory
	bankStages       []*bankStage
	parseBottomStage *bottomParser
	respondStage     *respondStage
}

// newPipelineMW creates the pipeline middleware of c and its stages.
func newPipelineMW(
	c *Comp,
	storage *mem.Storage,
) *pipelineMW {
	m := &pipelineMW{
		comp:    c,
		storage: storage,
	}

	spec := c.Spec

	m.intakeStage = &intake{cache: m}
	m.directoryStage = &directory{cache: m}
	for i := 0; i < spec.NumBanks; i++ {
		m.bankStages = append(m.bankStages, &bankStage{
			cache:          m,
			bankID:         i,
			numReqPerCycle: spec.NumReqPerCycle,
		})
	}
	m.parseBottomStage = &bottomParser{cache: m}
	m.respondStage = &respondStage{cache: m}

	return m
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

// Handle advances the cache pipeline by one cycle unless the cache is paused.
// When the cycle makes no progress, the cache sleeps until a pipeline item is
// due.
func (m *pipelineMW) Handle(_ timing.Event) bool {
	next := &m.comp.State
	cycle := m.currentCycle()

	if next.IsPaused {
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
	if cycle > next.DirPipeline.Cycle() {
		next.DirPipeline.AdvanceTo(cycle)
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
	if cycle > next.DirPipeline.Cycle() {
		next.DirPipeline.SkipTo(cycle)
	}
}

// scheduleWake asks for a tick at the next cycle a pipeline item becomes ready.
// Work blocked on anything else resumes through a port notification or through
// the progress of the stage that unblocks it.
func (m *pipelineMW) scheduleWake() {
	next := &m.comp.State
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
		consider(due - 1)
	}

	if found {
		m.comp.TickAt(timing.VTimeInPicoSec(wake) * m.comp.Spec.Freq.Period())
	}
}

func (m *pipelineMW) runPipeline() bool {
	madeProgress := false
	madeProgress = m.tickRespondStage() || madeProgress
	madeProgress = m.tickParseBottomStage() || madeProgress
	madeProgress = m.tickBankStage() || madeProgress
	madeProgress = m.tickDirectoryStage() || madeProgress
	madeProgress = m.tickIntakeStage() || madeProgress

	return madeProgress
}

func (m *pipelineMW) tickRespondStage() bool {
	madeProgress := false
	spec := m.comp.Spec
	for i := 0; i < spec.NumReqPerCycle; i++ {
		// Stop this cycle on backpressure or when no response is ready.
		// Port availability notifications schedule a later retry.
		if !m.respondStage.Tick() {
			break
		}
		madeProgress = true
	}

	return madeProgress
}

func (m *pipelineMW) tickParseBottomStage() bool {
	madeProgress := false

	spec := m.comp.Spec
	for i := 0; i < spec.NumReqPerCycle; i++ {
		madeProgress = m.parseBottomStage.Tick() || madeProgress
	}

	return madeProgress
}

func (m *pipelineMW) tickBankStage() bool {
	madeProgress := false
	for _, bs := range m.bankStages {
		madeProgress = bs.Tick() || madeProgress
	}

	return madeProgress
}

func (m *pipelineMW) tickDirectoryStage() bool {
	return m.directoryStage.Tick()
}

func (m *pipelineMW) tickIntakeStage() bool {
	madeProgress := false
	spec := m.comp.Spec
	for i := 0; i < spec.NumReqPerCycle; i++ {
		madeProgress = m.intakeStage.Tick() || madeProgress
	}

	return madeProgress
}
