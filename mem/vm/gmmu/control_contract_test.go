package gmmu

import (
	"testing"

	"github.com/sarchlab/akita/v5/mem/memcontrolprotocol"
	"github.com/sarchlab/akita/v5/mem/vm"
	akitasim "github.com/sarchlab/akita/v5/sim"
	"github.com/sarchlab/akita/v5/sim/messaging"
	"github.com/sarchlab/akita/v5/sim/modeling"
)

// TestControlContract verifies the GMMU satisfies the universal verbs.
// Invalidate and Flush respond as unsupported — the GMMU does not hold
// a private cache of memory.
func TestControlContract(t *testing.T) {
	build := func() *memcontrolprotocol.Harness {
		sim := akitasim.MakeBuilder().Build()

		spec := DefaultSpec()
		spec.DeviceID = 0
		spec.Latency = 1
		spec.Bottom = messaging.RemotePort("LowModule")

		comp := modeling.MakeBuilder[*Comp](spec).
			WithSimulation(sim).
			WithResources(Resources{
				PageTable: vm.NewPageTable(spec.Log2PageSize),
			}).
			Build("GMMU")
		compPorts := defaultPorts("GMMU")
		comp.BindPort("Top", compPorts.Top)
		comp.BindPort("Bottom", compPorts.Bottom)
		comp.BindPort("Control", compPorts.Control)

		for _, p := range allPorts(comp) {
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
				return len(comp.State.WalkingTranslations) == 0 &&
					len(comp.State.RemoteMemReqs) == 0
			},
		}
	}

	memcontrolprotocol.RunContract(t, "gmmu", build, memcontrolprotocol.Universal())
}

var _ memcontrolprotocol.Controllable = (*Comp)(nil)
