package mmuCache

import (
	"testing"

	"github.com/sarchlab/akita/v5/mem/internal/memcontroltest"
	akitasim "github.com/sarchlab/akita/v5/sim"
	"github.com/sarchlab/akita/v5/sim/messaging"
	"github.com/sarchlab/akita/v5/sim/modeling"
)

func TestControlContract(t *testing.T) {
	build := func() *memcontroltest.Harness {
		sim := akitasim.MakeBuilder().Build()

		spec := DefaultSpec()
		spec.NumBlocks = 1
		spec.NumLevels = 5
		spec.PageSize = 4096
		spec.Log2PageSize = 12
		spec.NumReqPerCycle = 4
		spec.LatencyPerLevel = 100

		comp := modeling.MakeBuilder[*Comp](spec).
			WithSimulation(sim).
			Build("MMUCache")
		compPorts := defaultPorts(sim, "MMUCache")
		comp.BindPort("Top", compPorts.Top)
		comp.BindPort("Bottom", compPorts.Bottom)
		comp.BindPort("Control", compPorts.Control)

		for _, p := range []messaging.Port{
			comp.Ports.Top, comp.Ports.Bottom, comp.Ports.Control,
		} {
			(&noopConn{}).BindPort(p)
		}
		if err := sim.Initialize(); err != nil {
			panic(err)
		}

		return &memcontroltest.Harness{
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
	memcontroltest.RunContract(t, "mmuCache", build, memcontroltest.TranslationCacheLike())
}

var _ memcontroltest.Controllable = (*Comp)(nil)
