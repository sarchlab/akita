package writethroughcache

import (
	"github.com/sarchlab/akita/v5/mem"
	"github.com/sarchlab/akita/v5/messaging"
	"github.com/sarchlab/akita/v5/modeling"
)

// pipelineMW holds all non-serializable infrastructure for the cache data
// pipeline. It implements the Tick method and delegates NamedHookable to comp.
type pipelineMW struct {
	comp *modeling.Component[Spec, State, Resources]

	topPort    messaging.Port
	bottomPort messaging.Port

	storage *mem.Storage

	intakeStage      *intake
	directoryStage   *directory
	bankStages       []*bankStage
	parseBottomStage *bottomParser
	respondStage     *respondStage
}

// findPort resolves an address to a remote port using data from Spec.
func (m *pipelineMW) findPort(address uint64) messaging.RemotePort {
	spec := m.comp.Spec()

	switch spec.AddressMapperType {
	case "single":
		if len(spec.RemotePortNames) > 0 {
			name := spec.RemotePortNames[0]
			if name != "" {
				return messaging.RemotePort(name)
			}
		}
	case "interleaved":
		if n := uint64(len(spec.RemotePortNames)); n > 0 {
			idx := address / spec.InterleavingSize % n
			name := spec.RemotePortNames[idx]
			if name != "" {
				return messaging.RemotePort(name)
			}
		}
	}

	panic("findPort: no valid address mapping for address; " +
		"Spec.AddressMapperType=" + spec.AddressMapperType)
}

// Tick updates the state of the cache pipeline.
func (m *pipelineMW) Tick() bool {
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
	spec := m.comp.Spec()
	for i := 0; i < spec.NumReqPerCycle; i++ {
		madeProgress = m.respondStage.Tick() || madeProgress
	}

	return madeProgress
}

func (m *pipelineMW) tickParseBottomStage() bool {
	madeProgress := false

	spec := m.comp.Spec()
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
	spec := m.comp.Spec()
	for i := 0; i < spec.NumReqPerCycle; i++ {
		madeProgress = m.intakeStage.Tick() || madeProgress
	}

	return madeProgress
}
