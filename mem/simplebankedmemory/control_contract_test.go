package simplebankedmemory

import (
	"testing"

	"github.com/sarchlab/akita/v5/hooking"
	"github.com/sarchlab/akita/v5/mem"
	"github.com/sarchlab/akita/v5/mem/control"
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
	build := func() *control.Harness {
		engine := timing.NewSerialEngine()
		storage := mem.NewStorage(1 * mem.MB)

		comp := MakeBuilder().
			WithRegistrar(modeling.NewStandaloneRegistrar(engine)).
			WithResources(Resources{Storage: storage}).
			Build("BankedMem")

		for _, name := range []string{"Top", "Control"} {
			(&noopConn{}).PlugIn(comp.GetPortByName(name))
		}

		return &control.Harness{
			Comp: comp,
			Ctrl: comp.GetPortByName("Control"),
			IsQuiescent: func() bool {
				for i := range comp.State.Banks {
					b := &comp.State.Banks[i]
					if len(b.Pipeline.Stages()) != 0 ||
						b.PostPipelineBuf.Size() != 0 {
						return false
					}
				}
				return true
			},
		}
	}

	control.RunContract(t, "simplebankedmemory", build, control.Universal())
}

var _ control.Controllable = (*Comp)(nil)
