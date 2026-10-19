package mmuCache

import (
	"testing"

	akitasim "github.com/sarchlab/akita/v5/sim"
	"github.com/sarchlab/akita/v5/sim/modeling"
	"github.com/sarchlab/akita/v5/sim/modeling/modelingtest"
	"github.com/stretchr/testify/require"
)

// TestComponentMatchesSource asserts that the inspector's static view of
// this package equals the runtime component type and DefaultSpec() result, so the two can never drift.
func TestComponentMatchesSource(t *testing.T) {
	modelingtest.CheckTicking(t, (*Comp)(nil), DefaultSpec())
}

// TestBuildRequiresBlocks checks that Build rejects a cache with no blocks.
func TestBuildRequiresBlocks(t *testing.T) {
	spec := DefaultSpec()
	spec.NumBlocks = 0

	require.PanicsWithValue(t, "mmuCache: Spec.NumBlocks must be > 0", func() {
		setupSim1 := akitasim.MakeBuilder().Build()
		builtComponent := modeling.MakeBuilder[*Comp](spec).
			WithSimulation(setupSim1).
			Build("MMUCache")
		builtComponentPorts := defaultPorts(setupSim1, "MMUCache")
		builtComponent.BindPort("Top", builtComponentPorts.Top)
		builtComponent.BindPort("Bottom", builtComponentPorts.Bottom)
		builtComponent.BindPort("Control", builtComponentPorts.Control)
		if err := setupSim1.Initialize(); err != nil {
			panic(err)
		}
	})
}
