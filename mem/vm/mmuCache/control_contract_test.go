package mmuCache

import (
	"testing"

	akitasim "github.com/sarchlab/akita/v5/sim"

	"github.com/sarchlab/akita/v5/mem/memcontrolprotocol"
	"github.com/sarchlab/akita/v5/sim/messaging"
)

func TestControlContract(t *testing.T) {
	build := func() *memcontrolprotocol.Harness {
		sim := akitasim.MakeBuilder().Build()

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
			Build("MMUCache")
		compPorts := defaultPorts("MMUCache")
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
