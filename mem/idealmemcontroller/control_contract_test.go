package idealmemcontroller

import (
	"testing"

	"github.com/sarchlab/akita/v5/mem"
	"github.com/sarchlab/akita/v5/mem/memcontrolprotocol"
	"github.com/sarchlab/akita/v5/messaging"
	"github.com/sarchlab/akita/v5/modeling"
	"github.com/sarchlab/akita/v5/timing"
)

// TestControlContract verifies the ideal memory controller satisfies
// the uniform control protocol. The component supports the four
// universal verbs; Invalidate and Flush must return "unsupported".
func TestControlContract(t *testing.T) {
	build := func() *memcontrolprotocol.Harness {
		engine := timing.NewSerialEngine()
		storage := mem.NewStorage(1 * mem.MB)
		spec := DefaultSpec()
		spec.Width = 1
		spec.Latency = 10
		spec.CacheLineSize = 64

		comp := MakeBuilder().
			WithRegistrar(modeling.NewStandaloneRegistrar(engine)).
			WithResources(Resources{Storage: storage}).
			WithSpec(spec).
			Build("MemCtrl")

		comp.AssignPort("Top",
			messaging.NewPort(comp, 16, 16, comp.Name()+".Top"))
		comp.AssignPort("Control",
			messaging.NewPort(comp, 16, 16, comp.Name()+".Control"))

		ctrl := comp.GetPortByName("Control")
		conn := &noopConn{}
		conn.PlugIn(comp.GetPortByName("Top"))
		conn.PlugIn(ctrl)

		return &memcontrolprotocol.Harness{
			Comp: comp,
			Ctrl: ctrl,
			IsQuiescent: func() bool {
				return len(comp.State.InflightTransactions) == 0
			},
		}
	}

	memcontrolprotocol.RunContract(t, "idealmemcontroller", build, memcontrolprotocol.Universal())
}

// Compile-time guard: the contract harness needs the component's Tick
// to satisfy memcontrolprotocol.Controllable.
var _ memcontrolprotocol.Controllable = (*Comp)(nil)
