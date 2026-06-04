package mmuCache

import (
	"testing"

	"github.com/sarchlab/akita/v5/mem/control"
	"github.com/sarchlab/akita/v5/modeling"
	"github.com/sarchlab/akita/v5/timing"
)

func TestControlContract(t *testing.T) {
	build := func() *control.Harness {
		engine := timing.NewSerialEngine()

		spec := DefaultSpec()
		spec.NumBlocks = 1
		spec.NumLevels = 5
		spec.PageSize = 4096
		spec.Log2PageSize = 12
		spec.NumReqPerCycle = 4
		spec.LatencyPerLevel = 100

		comp := MakeBuilder().
			WithRegistrar(modeling.NewStandaloneRegistrar(engine)).
			WithSpec(spec).
			Build("MMUCache")

		for _, name := range []string{"Top", "Bottom", "Control"} {
			(&noopConn{}).PlugIn(comp.GetPortByName(name))
		}

		return &control.Harness{
			Comp: comp,
			Ctrl: comp.GetPortByName("Control"),
		}
	}

	// An mmuCache caches translations (never dirty), so it supports
	// Invalidate but not Flush.
	control.RunContract(t, "mmuCache", build, control.TranslationCacheLike())
}

var _ control.Controllable = (*Comp)(nil)
