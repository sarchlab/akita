package tlb

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

		comp := Definition.Builder().
			WithSimulation(sim).
			WithResources(Resources{
				TranslationProviderMapper: &mem.SinglePortMapper{
					Port: messaging.RemotePort("MMU"),
				},
			}).
			WithPorts(defaultPorts("TLB")).
			Build("TLB")

		for _, p := range []messaging.Port{
			comp.Ports.Top, comp.Ports.Bottom, comp.Ports.Control,
		} {
			(&ccNoopConn{}).PlugIn(p)
		}

		return &memcontrolprotocol.Harness{
			Comp: comp,
			Sim:  sim,
			Ctrl: comp.Ports.Control,
			IsQuiescent: func() bool {
				return len(comp.State.MSHREntries) == 0 &&
					!comp.State.HasRespondingMSHR
			},
		}
	}

	// A TLB caches translations (never dirty), so it supports Invalidate
	// but not Flush.
	memcontrolprotocol.RunContract(t, "tlb", build, memcontrolprotocol.TranslationCacheLike())
}

var _ memcontrolprotocol.Controllable = (*Comp)(nil)
