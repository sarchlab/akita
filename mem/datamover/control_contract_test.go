package datamover

import (
	"testing"

	"github.com/sarchlab/akita/v5/hooking"
	"github.com/sarchlab/akita/v5/mem"
	"github.com/sarchlab/akita/v5/mem/control"
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
	build := func() *control.Harness {
		engine := timing.NewSerialEngine()

		spec := DefaultSpec()
		spec.BufferSize = 64
		spec.InsideByteGranularity = 8
		spec.OutsideByteGranularity = 8

		comp := MakeBuilder().
			WithRegistrar(modeling.NewStandaloneRegistrar(engine)).
			WithSpec(spec).
			WithResources(Resources{
				InsideMapper:  &mem.SinglePortMapper{Port: messaging.RemotePort("InsideMem")},
				OutsideMapper: &mem.SinglePortMapper{Port: messaging.RemotePort("OutsideMem")},
			}).
			Build("DataMover")

		for _, name := range []string{"Top", "Inside", "Outside", "Control"} {
			(&ccNoopConn{}).PlugIn(comp.GetPortByName(name))
		}

		return &control.Harness{
			Comp: comp,
			Ctrl: comp.GetPortByName("Control"),
		}
	}

	control.RunContract(t, "datamover", build, control.Universal())
}

var _ control.Controllable = (*Comp)(nil)
