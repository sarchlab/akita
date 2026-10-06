package mmuCache

import (
	"testing"

	"github.com/sarchlab/akita/v5/mem/memcontrolprotocol"
	"github.com/sarchlab/akita/v5/sim/messaging"
	"github.com/sarchlab/akita/v5/sim/modeling"
	"github.com/sarchlab/akita/v5/sim/timing"
)

func TestControlContract(t *testing.T) {
	build := func() *memcontrolprotocol.Harness {
		engine := timing.NewSerialEngine()
		sim := modeling.NewStandaloneSimulation(engine)

		spec := Definition.DefaultSpec
		spec.NumBlocks = 1
		spec.NumLevels = 5
		spec.PageSize = 4096
		spec.Log2PageSize = 12
		spec.NumReqPerCycle = 4
		spec.LatencyPerLevel = 100

		comp := Definition.Builder().
			WithSimulation(sim).
			WithSpec(spec).
			WithPorts(defaultPorts("MMUCache")).
			Build("MMUCache")

		for _, p := range []messaging.Port{
			comp.Ports.Top, comp.Ports.Bottom, comp.Ports.Control,
		} {
			(&noopConn{}).PlugIn(p)
		}

		return &memcontrolprotocol.Harness{
			Comp: comp,
			Sim:  sim,
			Ctrl: comp.Ports.Control,
			IsQuiescent: func() bool {
				return len(comp.State.OutstandingBottomReqs) == 0
			},
		}
	}

	// An mmuCache caches translations (never dirty), so it supports
	// Invalidate but not Flush.
	memcontrolprotocol.RunContract(t, "mmuCache", build, memcontrolprotocol.TranslationCacheLike())
}

var _ memcontrolprotocol.Controllable = (*Comp)(nil)
