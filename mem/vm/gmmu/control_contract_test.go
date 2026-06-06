package gmmu

import (
	"testing"

	"github.com/sarchlab/akita/v5/mem/control"
	"github.com/sarchlab/akita/v5/messaging"
	"github.com/sarchlab/akita/v5/modeling"
	"github.com/sarchlab/akita/v5/timing"
)

// TestControlContract verifies the GMMU satisfies the universal verbs.
// Invalidate and Flush respond as unsupported — the GMMU does not hold
// a private cache of memory.
func TestControlContract(t *testing.T) {
	build := func() *control.Harness {
		engine := timing.NewSerialEngine()
		spec := DefaultSpec()
		spec.DeviceID = 0
		spec.Latency = 1
		spec.LowModule = messaging.RemotePort("LowModule")

		comp := MakeBuilder().
			WithRegistrar(modeling.NewStandaloneRegistrar(engine)).
			WithSpec(spec).
			Build("GMMU")

		for _, name := range []string{"Top", "Bottom", "Control"} {
			(&noopConn{}).PlugIn(comp.GetPortByName(name))
		}

		return &control.Harness{
			Comp: comp,
			Ctrl: comp.GetPortByName("Control"),
			IsQuiescent: func() bool {
				return len(comp.State.WalkingTranslations) == 0 &&
					len(comp.State.RemoteMemReqs) == 0
			},
		}
	}

	control.RunContract(t, "gmmu", build, control.Universal())
}

var _ control.Controllable = (*Comp)(nil)
