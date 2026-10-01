package mmuCache

import (
	"testing"

	"github.com/sarchlab/akita/v5/modeling"
	"github.com/sarchlab/akita/v5/modeling/modelingtest"
	"github.com/sarchlab/akita/v5/timing"
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
		Definition.Builder().
			WithSimulation(modeling.NewStandaloneSimulation(timing.NewSerialEngine())).
			WithSpec(spec).
			WithPorts(defaultPorts("MMUCache")).
			Build("MMUCache")
	})
}
