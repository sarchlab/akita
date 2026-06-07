package tlb

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

		reg := modeling.NewStandaloneRegistrar(engine)
		comp := MakeBuilder().
			WithRegistrar(reg).
			WithResources(Resources{
				TranslationProviderMapper: &mem.SinglePortMapper{
					Port: messaging.RemotePort("MMU"),
				},
			}).
			Build("TLB")

		assignDefaultPorts(reg, comp)

		for _, name := range []string{"Top", "Bottom", "Control"} {
			(&ccNoopConn{}).PlugIn(comp.GetPortByName(name))
		}

		return &control.Harness{
			Comp: comp,
			Ctrl: comp.GetPortByName("Control"),
			IsQuiescent: func() bool {
				return len(comp.State.MSHREntries) == 0 &&
					!comp.State.HasRespondingMSHR
			},
		}
	}

	// A TLB caches translations (never dirty), so it supports Invalidate
	// but not Flush.
	control.RunContract(t, "tlb", build, control.TranslationCacheLike())
}

var _ control.Controllable = (*Comp)(nil)
