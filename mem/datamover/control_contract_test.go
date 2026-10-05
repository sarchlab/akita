package datamover

import (
	"testing"

	"github.com/sarchlab/akita/v5/mem"
	"github.com/sarchlab/akita/v5/mem/memcontrolprotocol"
	"github.com/sarchlab/akita/v5/sim/hooking"
	"github.com/sarchlab/akita/v5/sim/messaging"
	"github.com/sarchlab/akita/v5/sim/modeling"
	"github.com/sarchlab/akita/v5/sim/timing"
)

type ccNoopConn struct {
	hooking.HookableBase
}

func (c *ccNoopConn) Name() string                     { return "noopConn" }
func (c *ccNoopConn) PlugIn(port messaging.Port)       { port.SetConnection(c) }
func (c *ccNoopConn) Unplug(_ messaging.Port)          {}
func (c *ccNoopConn) NotifyAvailable(_ messaging.Port) {}
func (c *ccNoopConn) NotifySend()                      {}

func TestControlContract(t *testing.T) {
	build := func() *memcontrolprotocol.Harness {
		engine := timing.NewSerialEngine()
		sim := modeling.NewStandaloneSimulation(engine)

		spec := Definition.DefaultSpec
		spec.BufferSize = 64
		spec.InsideByteGranularity = 8
		spec.OutsideByteGranularity = 8

		comp := Definition.Builder().
			WithSimulation(sim).
			WithSpec(spec).
			WithResources(Resources{
				InsideMapper:  &mem.SinglePortMapper{Port: messaging.RemotePort("InsideMem")},
				OutsideMapper: &mem.SinglePortMapper{Port: messaging.RemotePort("OutsideMem")},
			}).
			WithPorts(makePorts("DataMover", 16, 16, 16, 16)).
			Build("DataMover")

		for _, p := range allPorts(comp) {
			(&ccNoopConn{}).PlugIn(p)
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
