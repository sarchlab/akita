package writethroughcache

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

		reg := modeling.NewStandaloneRegistrar(engine)
		comp := MakeBuilder().
			WithRegistrar(reg).
			WithSpec(spec).
			WithResources(Resources{
				Storage: storage,
				AddressMapper: &mem.SinglePortMapper{
					Port: messaging.RemotePort("LowerCache"),
				},
			}).
			Build("L1Cache")

		// Build declares the ports; assign every declared port instance
		// (the caller now chooses the buffer sizes) before the component
		// is ticked, then plug each into a no-op connection.
		for _, name := range []string{"Top", "Bottom", "Control"} {
			p := modeling.MakePortBuilder().
				WithRegistrar(reg).
				WithComponent(comp).
				WithSpec(modeling.PortSpec{BufSize: 4}).
				Build(name)
			comp.AssignPort(name, p)
			(&ccNoopConn{}).PlugIn(comp.GetPortByName(name))
		}

		return &memcontrolprotocol.Harness{
			Comp: comp,
			Ctrl: comp.GetPortByName("Control"),
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
	memcontrolprotocol.RunContract(t, "writethroughcache", build, matrix)
}

var _ memcontrolprotocol.Controllable = (*Comp)(nil)
