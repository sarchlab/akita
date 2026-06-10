package datamover

import (
	"testing"

	"github.com/sarchlab/akita/v5/hooking"
	"github.com/sarchlab/akita/v5/mem"
	"github.com/sarchlab/akita/v5/mem/memcontrolprotocol"
	"github.com/sarchlab/akita/v5/messaging"
	"github.com/sarchlab/akita/v5/modeling"
	"github.com/sarchlab/akita/v5/timing"
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

		spec := DefaultSpec()
		spec.BufferSize = 64
		spec.InsideByteGranularity = 8
		spec.OutsideByteGranularity = 8

		reg := modeling.NewStandaloneRegistrar(engine)

		comp := MakeBuilder().
			WithRegistrar(reg).
			WithSpec(spec).
			WithResources(Resources{
				InsideMapper:  &mem.SinglePortMapper{Port: messaging.RemotePort("InsideMem")},
				OutsideMapper: &mem.SinglePortMapper{Port: messaging.RemotePort("OutsideMem")},
			}).
			Build("DataMover")

		for _, name := range []string{"Top", "Inside", "Outside", "Control"} {
			p := modeling.MakePortBuilder().
				WithRegistrar(reg).
				WithComponent(comp).
				WithSpec(modeling.PortSpec{BufSize: 16}).
				Build(name)
			comp.AssignPort(name, p)
			(&ccNoopConn{}).PlugIn(comp.GetPortByName(name))
		}

		return &memcontrolprotocol.Harness{
			Comp: comp,
			Ctrl: comp.GetPortByName("Control"),
			IsQuiescent: func() bool {
				return !comp.State.CurrentTransaction.Active
			},
		}
	}

	memcontrolprotocol.RunContract(t, "datamover", build, memcontrolprotocol.Universal())
}

var _ memcontrolprotocol.Controllable = (*Comp)(nil)
