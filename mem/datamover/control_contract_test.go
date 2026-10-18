package datamover

import (
	"testing"

	"github.com/sarchlab/akita/v5/mem/memcontrolprotocol"
	akitasim "github.com/sarchlab/akita/v5/sim"
	"github.com/sarchlab/akita/v5/sim/hooking"
	"github.com/sarchlab/akita/v5/sim/messaging"
	"github.com/sarchlab/akita/v5/sim/modeling"
)

type ccNoopConn struct {
	hooking.HookableBase
}

func (c *ccNoopConn) Name() string                     { return "noopConn" }
func (c *ccNoopConn) BindPort(port messaging.Port)     { port.BindConnection(c) }
func (c *ccNoopConn) Unplug(_ messaging.Port)          {}
func (c *ccNoopConn) NotifyAvailable(_ messaging.Port) {}
func (c *ccNoopConn) NotifySend(_ messaging.Port)      {}

func TestControlContract(t *testing.T) {
	build := func() *memcontrolprotocol.Harness {
		sim := akitasim.MakeBuilder().Build()

		spec := DefaultSpec()
		spec.BufferSize = 64
		spec.InsideByteGranularity = 8
		spec.OutsideByteGranularity = 8

		spec.Inside.Destinations = []messaging.RemotePort{messaging.RemotePort("InsideMem")}

		spec.Outside.Destinations = []messaging.RemotePort{messaging.RemotePort("OutsideMem")}
		comp := modeling.MakeBuilder[*Comp](spec).
			WithSimulation(sim).
			Build("DataMover")
		compPorts := makePorts("DataMover", 16, 16, 16, 16)
		comp.BindPort("Top", compPorts.Top)
		comp.BindPort("Inside", compPorts.Inside)
		comp.BindPort("Outside", compPorts.Outside)
		comp.BindPort("Control", compPorts.Control)

		for _, p := range allPorts(comp) {
			(&ccNoopConn{}).BindPort(p)
		}
		if err := sim.Initialize(); err != nil {
			panic(err)
		}

		return &memcontrolprotocol.Harness{
			Comp: comp,
			Sim:  sim,
			Ctrl: comp.Ports.Control,
			IsQuiescent: func() bool {
				return !comp.State.CurrentTransaction.Active
			},
		}
	}

	memcontrolprotocol.RunContract(t, "datamover", build, memcontrolprotocol.Universal())
}

var _ memcontrolprotocol.Controllable = (*Comp)(nil)
