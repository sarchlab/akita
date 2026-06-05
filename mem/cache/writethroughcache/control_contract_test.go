package writethroughcache

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
		spec.TopPortBufferSize = 4
		spec.BottomPortBufferSize = 4
		spec.ControlPortBufferSize = 4

		comp := MakeBuilder().
			WithRegistrar(modeling.NewStandaloneRegistrar(engine)).
			WithSpec(spec).
			WithResources(Resources{
				Storage: storage,
				AddressMapper: &mem.SinglePortMapper{
					Port: messaging.RemotePort("LowerCache"),
				},
			}).
			Build("L1Cache")

		for _, name := range []string{"Top", "Bottom", "Control"} {
			(&ccNoopConn{}).PlugIn(comp.GetPortByName(name))
		}

		return &control.Harness{
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
	matrix := control.CacheLike()
	control.RunContract(t, "writethroughcache", build, matrix)
}

var _ control.Controllable = (*Comp)(nil)
