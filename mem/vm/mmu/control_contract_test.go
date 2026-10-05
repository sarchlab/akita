package mmu

import (
	"testing"

	"github.com/sarchlab/akita/v5/mem/memcontrolprotocol"
	"github.com/sarchlab/akita/v5/mem/vm"
	"github.com/sarchlab/akita/v5/simulation/messaging"
	"github.com/sarchlab/akita/v5/simulation/modeling"
	"github.com/sarchlab/akita/v5/simulation/timing"
)

// TestControlContract verifies the MMU satisfies the universal verbs.
// Invalidate and Flush respond as unsupported — the MMU does not hold
// a private cache of memory.
func TestControlContract(t *testing.T) {
	build := func() *memcontrolprotocol.Harness {
		engine := timing.NewSerialEngine()
		sim := modeling.NewStandaloneSimulation(engine)

		comp := Definition.Builder().
			WithSimulation(sim).
			WithSpec(Definition.DefaultSpec).
			WithResources(Resources{PageTable: vm.NewPageTable(12)}).
			WithPorts(makePorts("MMU", 16)).
			Build("MMU")

		for _, p := range []messaging.Port{comp.Ports.Top, comp.Ports.Control} {
			(&noopConn{}).PlugIn(p)
		}

		return &memcontrolprotocol.Harness{
			Comp: comp,
			Sim:  sim,
			Ctrl: comp.Ports.Control,
			IsQuiescent: func() bool {
				return len(comp.State.WalkingTranslations) == 0
			},
		}
	}

	memcontrolprotocol.RunContract(t, "mmu", build, memcontrolprotocol.Universal())
}

var _ memcontrolprotocol.Controllable = (*Comp)(nil)
