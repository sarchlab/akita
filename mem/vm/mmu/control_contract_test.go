package mmu

import (
	"testing"

	"github.com/sarchlab/akita/v5/mem/control"
	"github.com/sarchlab/akita/v5/modeling"
	"github.com/sarchlab/akita/v5/timing"
)

// TestControlContract verifies the MMU satisfies the universal verbs.
// Invalidate and Flush respond as unsupported — the MMU does not hold
// a private cache of memory.
func TestControlContract(t *testing.T) {
	build := func() *control.Harness {
		engine := timing.NewSerialEngine()
		spec := DefaultSpec()
		spec.TopPortBufferSize = 16
		spec.CtrlPortBufferSize = 4

		comp := MakeBuilder().
			WithRegistrar(modeling.NewStandaloneRegistrar(engine)).
			WithSpec(spec).
			Build("MMU")

		for _, name := range []string{"Top", "Control"} {
			(&noopConn{}).PlugIn(comp.GetPortByName(name))
		}

		return &control.Harness{
			Comp: comp,
			Ctrl: comp.GetPortByName("Control"),
			IsQuiescent: func() bool {
				return len(comp.State.WalkingTranslations) == 0
			},
		}
	}

	control.RunContract(t, "mmu", build, control.Universal())
}

var _ control.Controllable = (*Comp)(nil)
