package idealmemcontroller

import (
	"testing"

	"github.com/sarchlab/akita/v5/mem"
	"github.com/sarchlab/akita/v5/mem/control"
	"github.com/sarchlab/akita/v5/modeling"
	"github.com/sarchlab/akita/v5/timing"
)

// TestControlContract verifies the ideal memory controller satisfies
// the uniform control protocol. The component supports the four
// universal verbs; Invalidate and Flush must return "unsupported".
func TestControlContract(t *testing.T) {
	build := func() *control.Harness {
		engine := timing.NewSerialEngine()
		storage := mem.NewStorage(1 * mem.MB)
		spec := DefaultSpec()
		spec.Width = 1
		spec.Latency = 10
		spec.CacheLineSize = 64
		spec.TopPortBufferSize = 16
		spec.CtrlPortBufferSize = 16

		comp := MakeBuilder().
			WithRegistrar(modeling.NewStandaloneRegistrar(engine)).
			WithResources(Resources{Storage: storage}).
			WithSpec(spec).
			Build("MemCtrl")

		ctrl := comp.GetPortByName("Control")
		conn := &noopConn{}
		conn.PlugIn(comp.GetPortByName("Top"))
		conn.PlugIn(ctrl)

		return &control.Harness{
			Comp: comp,
			Ctrl: ctrl,
			IsQuiescent: func() bool {
				return len(comp.State.InflightTransactions) == 0
			},
		}
	}

	control.RunContract(t, "idealmemcontroller", build, control.Universal())
}

// Compile-time guard: the contract harness needs the component's Tick
// to satisfy control.Controllable.
var _ control.Controllable = (*Comp)(nil)