package idealmemcontroller

import (
	"testing"

	"github.com/sarchlab/akita/v5/mem"
	"github.com/sarchlab/akita/v5/mem/memcontrolprotocol"
	"github.com/sarchlab/akita/v5/modeling"
	"github.com/sarchlab/akita/v5/timing"
)

// TestControlContract verifies the ideal memory controller satisfies
// the uniform control protocol. The component supports the four
// universal verbs; Invalidate and Flush must return "unsupported".
func TestControlContract(t *testing.T) {
	build := func() *memcontrolprotocol.Harness {
		engine := timing.NewSerialEngine()
		sim := modeling.NewStandaloneSimulation(engine)
		storage := mem.NewStorage(1 * mem.MB)
		spec := Definition.DefaultSpec
		spec.Width = 1
		spec.Latency = 10
		spec.CacheLineSize = 64

		comp := Definition.Builder().
			WithSimulation(sim).
			WithResources(Resources{Storage: storage}).
			WithSpec(spec).
			WithPorts(makePorts("MemCtrl", 16)).
			Build("MemCtrl")

		ctrl := comp.Ports.Control
		conn := &noopConn{}
		conn.PlugIn(comp.Ports.Top)
		conn.PlugIn(ctrl)

		return &memcontrolprotocol.Harness{
			Comp: comp,
			Ctrl: ctrl,
			IsQuiescent: func() bool {
				return len(comp.State.InflightTransactions) == 0
			},
		}
	}

	memcontrolprotocol.RunContract(t, "idealmemcontroller", build, memcontrolprotocol.Universal())
}

// Compile-time guard: the contract harness needs the component's Handle
// to satisfy memcontrolprotocol.Controllable.
var _ memcontrolprotocol.Controllable = (*Comp)(nil)
