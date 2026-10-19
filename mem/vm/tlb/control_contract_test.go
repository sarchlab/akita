package tlb

import (
	"testing"

	"github.com/sarchlab/akita/v5/mem/internal/memcontroltest"

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
	build := func() *memcontroltest.Harness {
		sim := akitasim.MakeBuilder().Build()

		routingSpec1 := DefaultSpec()

		routingSpec1.Bottom.Destinations = []messaging.RemotePort{messaging.RemotePort("MMU")}
		comp := modeling.MakeBuilder[*Comp](routingSpec1).
			WithSimulation(sim).
			Build("TLB")
		compPorts := defaultPorts("TLB")
		comp.BindPort("Top", compPorts.Top)
		comp.BindPort("Bottom", compPorts.Bottom)
		comp.BindPort("Control", compPorts.Control)

		for _, p := range []messaging.Port{
			comp.Ports.Top, comp.Ports.Bottom, comp.Ports.Control,
		} {
			(&ccNoopConn{}).BindPort(p)
		}
		if err := sim.Initialize(); err != nil {
			panic(err)
		}

		return &memcontroltest.Harness{
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
	memcontroltest.RunContract(t, "tlb", build, memcontroltest.TranslationCacheLike())
}

var _ memcontroltest.Controllable = (*Comp)(nil)
