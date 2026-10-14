package mmuCache

import (
	"testing"

	akitasim "github.com/sarchlab/akita/v5/sim"

	"github.com/sarchlab/akita/v5/sim/modeling/modelingtest"
	"github.com/stretchr/testify/require"
)

// TestDefinitionMatchesSource asserts that the inspector's static view of
// this package equals the runtime Definition, so the two can never drift.
func TestDefinitionMatchesSource(t *testing.T) {
	modelingtest.CheckTicking(t, Definition)
}

// TestBuildRequiresBlocks checks that Build rejects a cache with no blocks.
func TestBuildRequiresBlocks(t *testing.T) {
	spec := Definition.DefaultSpec
	spec.NumBlocks = 0

	require.PanicsWithValue(t, "mmuCache: Spec.NumBlocks must be > 0", func() {
		setupSim1 := akitasim.MakeBuilder().Build()
		builtComponent := Definition.Builder().
			WithSimulation(setupSim1).
			WithSpec(spec).
			Build("MMUCache")
		builtComponentPorts := defaultPorts("MMUCache")
		builtComponent.BindPort("Top", builtComponentPorts.Top)
		builtComponent.BindPort("Bottom", builtComponentPorts.Bottom)
		builtComponent.BindPort("Control", builtComponentPorts.Control)
		if err := setupSim1.Initialize(); err != nil {
			panic(err)
		}
	})
}
