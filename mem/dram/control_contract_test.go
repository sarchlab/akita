package dram

import (
	"testing"

	"github.com/sarchlab/akita/v5/hooking"
	"github.com/sarchlab/akita/v5/mem"
	"github.com/sarchlab/akita/v5/mem/memcontrolprotocol"
	"github.com/sarchlab/akita/v5/messaging"
	"github.com/sarchlab/akita/v5/modeling"
	"github.com/sarchlab/akita/v5/timing"
)

type noopConn struct {
	hooking.HookableBase
}

func (c *noopConn) Name() string                     { return "noopConn" }
func (c *noopConn) PlugIn(port messaging.Port)       { port.SetConnection(c) }
func (c *noopConn) Unplug(_ messaging.Port)          {}
func (c *noopConn) NotifyAvailable(_ messaging.Port) {}
func (c *noopConn) NotifySend()                      {}

func TestControlContract(t *testing.T) {
	build := func() *memcontrolprotocol.Harness {
		engine := timing.NewSerialEngine()
		storage := mem.NewStorage(1 * mem.MB)

		reg := modeling.NewStandaloneRegistrar(engine)
		comp := MakeBuilder().
			WithRegistrar(reg).
			WithResources(Resources{Storage: storage}).
			Build("DRAM")

		for _, name := range []string{"Top", "Control"} {
			p := modeling.MakePortBuilder().
				WithRegistrar(reg).
				WithComponent(comp).
				WithSpec(modeling.PortSpec{BufSize: 16}).
				Build(name)
			comp.AssignPort(name, p)
			(&noopConn{}).PlugIn(comp.GetPortByName(name))
		}

		return &memcontrolprotocol.Harness{
			Comp: comp,
			Ctrl: comp.GetPortByName("Control"),
			IsQuiescent: func() bool {
				return len(comp.State.Transactions) == 0
			},
		}
	}

	memcontrolprotocol.RunContract(t, "dram", build, memcontrolprotocol.Universal())
}

var _ memcontrolprotocol.Controllable = (*Comp)(nil)
