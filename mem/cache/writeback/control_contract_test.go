package writeback

import (
	"testing"

	"github.com/sarchlab/akita/v5/mem"
	"github.com/sarchlab/akita/v5/mem/memcontrolprotocol"
	"github.com/sarchlab/akita/v5/simulation/hooking"
	"github.com/sarchlab/akita/v5/simulation/messaging"
	"github.com/sarchlab/akita/v5/simulation/modeling"
	"github.com/sarchlab/akita/v5/simulation/timing"
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
		storage := mem.NewStorage(1 * mem.MB)

		spec := Definition.DefaultSpec
		spec.TotalByteSize = 64 * 1024
		spec.NumBanks = 1
		spec.NumMSHREntry = 4
		spec.NumReqPerCycle = 1
		spec.WayAssociativity = 2
		spec.Log2BlockSize = 6
		spec.BankLatency = 1
		spec.DirLatency = 1

		comp := Definition.Builder().
			WithSimulation(sim).
			WithSpec(spec).
			WithResources(Resources{
				Storage: storage,
				AddressToPortMapper: &mem.SinglePortMapper{
					Port: messaging.RemotePort("LowerCache"),
				},
			}).
			WithPorts(makePorts("L1Cache", 4)).
			Build("L1Cache")
		plugNoopConn(comp)

		return &memcontrolprotocol.Harness{
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
	matrix := memcontrolprotocol.CacheLike()
	memcontrolprotocol.RunContract(t, "writeback", build, matrix)
}

var _ memcontrolprotocol.Controllable = (*Comp)(nil)
