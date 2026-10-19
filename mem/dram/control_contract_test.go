package dram

import (
	"testing"

	"github.com/sarchlab/akita/v5/mem/internal/memcontroltest"

	"github.com/sarchlab/akita/v5/mem"
	akitasim "github.com/sarchlab/akita/v5/sim"
	"github.com/sarchlab/akita/v5/sim/hooking"
	"github.com/sarchlab/akita/v5/sim/messaging"
	"github.com/sarchlab/akita/v5/sim/modeling"
)

type noopConn struct {
	hooking.HookableBase
}

func (c *noopConn) Name() string                     { return "noopConn" }
func (c *noopConn) BindPort(port messaging.Port)     { port.BindConnection(c) }
func (c *noopConn) Unplug(_ messaging.Port)          {}
func (c *noopConn) NotifyAvailable(_ messaging.Port) {}
func (c *noopConn) NotifySend(_ messaging.Port)      {}

func TestControlContract(t *testing.T) {
	build := func() *memcontroltest.Harness {
		sim := akitasim.MakeBuilder().Build()

		storage := mem.NewStorage(1 * mem.MB)

		comp := modeling.MakeBuilder[*Comp](DefaultSpec()).
			WithSimulation(sim).
			WithResources(Resources{Storage: storage}).
			Build("DRAM")
		compPorts := defaultPorts("DRAM", 16)
		comp.BindPort("Top", compPorts.Top)
		comp.BindPort("Control", compPorts.Control)

		for _, p := range []messaging.Port{comp.Ports.Top, comp.Ports.Control} {
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
				return len(comp.State.Transactions) == 0
			},
		}
	}

	memcontroltest.RunContract(t, "dram", build, memcontroltest.Universal())
}

var _ memcontroltest.Controllable = (*Comp)(nil)
