package writeback

import (
	"github.com/sarchlab/akita/v5/mem"
	"github.com/sarchlab/akita/v5/messaging"
	"github.com/sarchlab/akita/v5/timing"
)

// pipelineMW runs the writeback cache's data pipeline. It holds only
// references: the component, the storage and address mapper resolved at build
// time, and the pipeline stages. All mutable state is in comp.State.
type pipelineMW struct {
	comp *Comp

	storage       *mem.Storage
	addressMapper mem.AddressToPortMapper

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

	numBanks := spec.numBanks()
	m.bankStages = make([]*bankStage, numBanks)
	for i := 0; i < numBanks; i++ {
		m.bankStages[i] = &bankStage{
			cache:         m,
			bankID:        i,
			pipelineWidth: spec.laneWidth(),
		}
	}

	m.mshrStage = &mshrStage{cache: m}
	m.writeBuffer = &writeBufferStage{cache: m}
}

// GetSpec returns the immutable specification.
func (m *pipelineMW) GetSpec() Spec {
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
	if m.addressMapper == nil {
		panic("writeback: no address mapper; set Resources.AddressToPortMapper, " +
			"or Spec.AddressMapperType with Resources.RemotePorts")
	}

	return m.addressMapper.Find(address)
}

// Handle runs one cycle of the cache pipeline, unless the cache is paused.
func (m *pipelineMW) Handle(_ timing.Event) bool {
	next := &m.comp.State
	madeProgress := false

	if cacheState(next.CacheState) != cacheStatePaused {
		madeProgress = m.runPipeline() || madeProgress
	}

	return madeProgress
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
