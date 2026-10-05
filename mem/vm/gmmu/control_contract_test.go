package gmmu

import (
	"testing"

	"github.com/sarchlab/akita/v5/mem/memcontrolprotocol"
	"github.com/sarchlab/akita/v5/mem/vm"
	"github.com/sarchlab/akita/v5/messaging"
	"github.com/sarchlab/akita/v5/modeling"
	"github.com/sarchlab/akita/v5/timing"
)

// TestControlContract verifies the GMMU satisfies the universal verbs.
// Invalidate and Flush respond as unsupported — the GMMU does not hold
// a private cache of memory.
func TestControlContract(t *testing.T) {
	build := func() *memcontrolprotocol.Harness {
		engine := timing.NewSerialEngine()
		sim := modeling.NewStandaloneSimulation(engine)
		spec := Definition.DefaultSpec
		spec.DeviceID = 0
		spec.Latency = 1
		spec.LowModule = messaging.RemotePort("LowModule")

		comp := Definition.Builder().
			WithSimulation(sim).
			WithSpec(spec).
			WithResources(Resources{
				PageTable: vm.NewPageTable(spec.Log2PageSize),
			}).
			WithPorts(defaultPorts("GMMU")).
			Build("GMMU")

		for _, p := range allPorts(comp) {
			(&noopConn{}).PlugIn(p)
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
