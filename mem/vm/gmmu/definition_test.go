package gmmu

import (
	"testing"

	"github.com/sarchlab/akita/v5/simulation/modeling"
	"github.com/sarchlab/akita/v5/simulation/modeling/modelingtest"
	"github.com/sarchlab/akita/v5/simulation/timing"
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
		Definition.Builder().
			WithSimulation(modeling.NewStandaloneSimulation(timing.NewSerialEngine())).
			WithPorts(defaultPorts("GMMU")).
			Build("GMMU")
	})
}
