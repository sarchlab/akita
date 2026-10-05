package writethroughcache

import (
	"github.com/sarchlab/akita/v5/mem"
	"github.com/sarchlab/akita/v5/simulation/messaging"
	"github.com/sarchlab/akita/v5/simulation/timing"
)

// pipelineMW runs the cache data pipeline. It holds only references: the
// component, the storage and address mapper from Resources, and the pipeline
// stages, which keep their mutable data in State.
type pipelineMW struct {
	comp *Comp

	storage       *mem.Storage
	addressMapper mem.AddressToPortMapper

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
	addressMapper mem.AddressToPortMapper,
) *pipelineMW {
	m := &pipelineMW{
		comp:          c,
		storage:       storage,
		addressMapper: addressMapper,
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
	if m.addressMapper == nil {
		panic("writethroughcache: no address mapper; set Resources.AddressMapper, " +
			"or Spec.AddressMapperType with Resources.RemotePorts")
	}

	return m.addressMapper.Find(address)
}

// Handle advances the cache pipeline by one cycle unless the cache is paused.
func (m *pipelineMW) Handle(_ timing.Event) bool {
	next := &m.comp.State
	madeProgress := false

	if !next.IsPaused {
		madeProgress = m.runPipeline() || madeProgress
	}

	return madeProgress
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
		madeProgress = m.respondStage.Tick() || madeProgress
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
