package writeback

import (
	"testing"

	"github.com/sarchlab/akita/v5/mem"
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
		compPorts := makePorts(sim, "L1Cache", 4)
		comp.BindPort("Top", compPorts.Top)
		comp.BindPort("Bottom", compPorts.Bottom)
		comp.BindPort("Control", compPorts.Control)
		plugNoopConn(comp)
		if err := sim.Initialize(); err != nil {
			panic(err)
		}

		return &memcontroltest.Harness{
			Comp: comp,
			Sim:  sim,
			Ctrl: comp.Ports.Control,
			IsQuiescent: func() bool {
				return cacheIsQuiescent(&comp.State)
			},
		}
	}

	// Phase 3 adds Invalidate and the address/PID filter on Flush, so the
	// writeback cache now satisfies the full cache-like matrix: the four
	// universal verbs plus the two conditional verbs (Invalidate, Flush).
	matrix := memcontroltest.CacheLike()
	memcontroltest.RunContract(t, "writeback", build, matrix)
}

var _ memcontroltest.Controllable = (*Comp)(nil)
