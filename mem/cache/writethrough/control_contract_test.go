package writethrough

import (
	"testing"

	"github.com/sarchlab/akita/v5/mem"
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

		storage := mem.NewStorage(1 * mem.MB)

		spec := DefaultSpec()
		spec.TotalByteSize = 64 * 1024
		spec.NumBanks = 1
		spec.NumMSHREntry = 4
		spec.NumReqPerCycle = 1
		spec.WayAssociativity = 2
		spec.Log2BlockSize = 6
		spec.BankLatency = 1
		spec.DirLatency = 1

		spec.Bottom.Destinations = []messaging.RemotePort{messaging.RemotePort("LowerCache")}
		comp := modeling.MakeBuilder[*Comp](spec).
			WithSimulation(sim).
			WithResources(Resources{Storage: storage}).
			Build("L1Cache")
		compPorts := makePorts("L1Cache", 4)
		comp.BindPort("Top", compPorts.Top)
		comp.BindPort("Bottom", compPorts.Bottom)
		comp.BindPort("Control", compPorts.Control)

		// Plug each port into a no-op connection before the component is
		// ticked.
		for _, p := range []messaging.Port{
			comp.Ports.Top, comp.Ports.Bottom, comp.Ports.Control,
		} {
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
				for i := range comp.State.Transactions {
					if !comp.State.Transactions[i].Removed {
						return false
					}
				}
				return true
			},
		}
	}

	// Phase 3 completes the writethrough control surface: the universal
	// verbs plus both conditional verbs (Invalidate drops clean blocks,
	// Flush is a no-op because writethrough holds no dirty data).
	matrix := memcontrolprotocol.CacheLike()
	memcontrolprotocol.RunContract(t, "writethrough", build, matrix)
}

var _ memcontrolprotocol.Controllable = (*Comp)(nil)
