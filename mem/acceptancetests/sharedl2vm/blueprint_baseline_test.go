package main

import (
	"fmt"
	"github.com/sarchlab/akita/v5/blueprint"
	"github.com/sarchlab/akita/v5/mem/internal/memaccessagent"
	"github.com/sarchlab/akita/v5/mem/vm"
	"github.com/sarchlab/akita/v5/sim"
	"github.com/sarchlab/akita/v5/sim/modeling"
	"github.com/stretchr/testify/require"
	"testing"
)

// Captured from the original hand-built program at 1d8104ba: four agents,
// seed 42, 100 reads and writes each, 65536-byte private ranges: 3683000 ps.
func TestBlueprintPreservesSimulation(t *testing.T) {
	for _, parallel := range []bool{false, true} {
		name := "serial"
		if parallel {
			name = "parallel"
		}
		t.Run(name, func(t *testing.T) {
			b := sim.MakeBuilder()
			if parallel {
				b = b.WithParallelEngine()
			}
			s := b.Build()
			defer s.Terminate()
			s.Add("", blueprint.NewDomain[platformSpec, modeling.None](platformSpec{Seed: 42, NumAccess: 100,
				MaxAddress: 65536}, declarePlatform))
			require.NoError(t, s.Initialize())
			setupPageTable(numAgents*65536, blueprint.Lookup[*vm.PageTable](s, "PageTable"))
			agents := make([]*memaccessagent.Comp, numAgents)
			for i := range agents {
				agents[i] = blueprint.Lookup[*memaccessagent.Comp](s, fmt.Sprintf("Chain[%d].Agent", i))
				agents[i].TickLater()
			}
			require.NoError(t, s.Run())
			for _, a := range agents {
				require.Zero(t, a.State.ReadLeft)
				require.Zero(t, a.State.WriteLeft)
				require.Empty(t, a.State.PendingReadReq)
				require.Empty(t, a.State.PendingWriteReq)
			}
			if !parallel {
				require.EqualValues(t, 3683000, s.Engine().CurrentTime())
			}
			t.Logf("sharedl2vm %s: four agents complete 100 reads and 100 writes each at %d ps", name,
				s.Engine().CurrentTime())
		})
	}
}
