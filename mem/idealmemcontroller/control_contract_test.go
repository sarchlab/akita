package idealmemcontroller

import (
	"testing"

	"github.com/sarchlab/akita/v5/mem"
	"github.com/sarchlab/akita/v5/mem/memcontrolprotocol"
	"github.com/sarchlab/akita/v5/sim/modeling"
	"github.com/sarchlab/akita/v5/sim/timing"
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
			Build("MemCtrl")
		compPorts := makePorts("MemCtrl", 16)
		comp.BindPort("Top", compPorts.Top)
		comp.BindPort("Control", compPorts.Control)

		ctrl := comp.Ports.Control
		conn := &noopConn{}
		conn.BindPort(comp.Ports.Top)
		conn.BindPort(ctrl)
		if err := sim.Initialize(); err != nil {
			panic(err)
		}

		return &memcontrolprotocol.Harness{
			Comp: comp,
			Sim:  sim,
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
