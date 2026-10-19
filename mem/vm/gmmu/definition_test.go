package gmmu

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

// TestBuildRequiresPageTable checks that Build rejects a GMMU without a page
// table instead of failing on the first walk.
func TestBuildRequiresPageTable(t *testing.T) {
	require.PanicsWithValue(t, "gmmu: Resources.PageTable is required", func() {
		setupSim1 := akitasim.MakeBuilder().Build()
		builtComponent := modeling.MakeBuilder[*Comp](DefaultSpec()).
			WithSimulation(setupSim1).
			Build("GMMU")
		builtComponentPorts := defaultPorts(setupSim1, "GMMU")
		builtComponent.BindPort("Top", builtComponentPorts.Top)
		builtComponent.BindPort("Bottom", builtComponentPorts.Bottom)
		builtComponent.BindPort("Control", builtComponentPorts.Control)
		if err := setupSim1.Initialize(); err != nil {
			panic(err)
		}
	})
}
