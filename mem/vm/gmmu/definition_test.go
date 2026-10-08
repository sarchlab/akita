package gmmu

import (
	"testing"

	"github.com/sarchlab/akita/v5/sim/modeling"
	"github.com/sarchlab/akita/v5/sim/modeling/modelingtest"
	"github.com/sarchlab/akita/v5/sim/timing"
	"github.com/stretchr/testify/require"
)

// TestDefinitionMatchesSource asserts that the inspector's static view of
// this package equals the runtime Definition, so the two can never drift.
func TestDefinitionMatchesSource(t *testing.T) {
	modelingtest.CheckTicking(t, Definition)
}

// TestBuildRequiresPageTable checks that Build rejects a GMMU without a page
// table instead of failing on the first walk.
func TestBuildRequiresPageTable(t *testing.T) {
	require.PanicsWithValue(t, "gmmu: Resources.PageTable is required", func() {
		setupSim1 := modeling.NewStandaloneSimulation(timing.NewSerialEngine())
		builtComponent := Definition.Builder().
			WithSimulation(setupSim1).
			Build("GMMU")
		builtComponentPorts := defaultPorts("GMMU")
		builtComponent.BindPort("Top", builtComponentPorts.Top)
		builtComponent.BindPort("Bottom", builtComponentPorts.Bottom)
		builtComponent.BindPort("Control", builtComponentPorts.Control)
		if err := setupSim1.Initialize(); err != nil {
			panic(err)
		}
	})
}
