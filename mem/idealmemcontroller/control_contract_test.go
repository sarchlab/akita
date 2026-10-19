package idealmemcontroller

import (
	"testing"

	"github.com/sarchlab/akita/v5/mem/internal/memcontroltest"

	"github.com/sarchlab/akita/v5/mem"
	akitasim "github.com/sarchlab/akita/v5/sim"
	"github.com/sarchlab/akita/v5/sim/modeling"
)

// TestControlContract verifies the ideal memory controller satisfies
// the uniform control protocol. The component supports the four
// universal verbs; Invalidate and Flush must return "unsupported".
func TestControlContract(t *testing.T) {
	build := func() *memcontroltest.Harness {
		sim := akitasim.MakeBuilder().Build()

		storage := mem.NewStorage(1 * mem.MB)
		spec := DefaultSpec()
		spec.Width = 1
		spec.Latency = 10

		comp := modeling.MakeBuilder[*Comp](spec).
			WithSimulation(sim).
			WithResources(Resources{Storage: storage}).
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

		return &memcontroltest.Harness{
			Comp: comp,
			Sim:  sim,
			Ctrl: ctrl,
			IsQuiescent: func() bool {
				return len(comp.State.InflightTransactions) == 0
			},
		}
	}

	memcontroltest.RunContract(t, "idealmemcontroller", build, memcontroltest.Universal())
}

// Compile-time guard: the contract harness needs the component's Handle
// to satisfy memcontroltest.Controllable.
var _ memcontroltest.Controllable = (*Comp)(nil)
