package mmuCache

import (
	"testing"

	"github.com/sarchlab/akita/v5/mem/memcontrolprotocol"
	"github.com/sarchlab/akita/v5/modeling"
	"github.com/sarchlab/akita/v5/timing"
)

func TestControlContract(t *testing.T) {
	build := func() *memcontrolprotocol.Harness {
		engine := timing.NewSerialEngine()

		spec := DefaultSpec()
		spec.NumBlocks = 1
		spec.NumLevels = 5
		spec.PageSize = 4096
		spec.Log2PageSize = 12
		spec.NumReqPerCycle = 4
		spec.LatencyPerLevel = 100

		reg := modeling.NewStandaloneRegistrar(engine)
		comp := MakeBuilder().
			WithRegistrar(reg).
			WithSpec(spec).
			Build("MMUCache")

		assignDefaultPorts(reg, comp)

		for _, name := range []string{"Top", "Bottom", "Control"} {
			(&noopConn{}).PlugIn(comp.GetPortByName(name))
		}

		return &memcontrolprotocol.Harness{
			Comp: comp,
			Ctrl: comp.GetPortByName("Control"),
			IsQuiescent: func() bool {
				return len(comp.State.OutstandingBottomReqs) == 0
			},
		}
	}

	// An mmuCache caches translations (never dirty), so it supports
	// Invalidate but not Flush.
	memcontrolprotocol.RunContract(t, "mmuCache", build, memcontrolprotocol.TranslationCacheLike())
}

var _ memcontrolprotocol.Controllable = (*Comp)(nil)
