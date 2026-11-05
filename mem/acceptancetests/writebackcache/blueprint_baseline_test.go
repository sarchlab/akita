package main

import (
	"testing"

	"github.com/sarchlab/akita/v5/blueprint"
	workload "github.com/sarchlab/akita/v5/mem/internal/accessworkload"
	"github.com/sarchlab/akita/v5/mem/internal/memaccessagent"
	"github.com/sarchlab/akita/v5/sim"
	"github.com/sarchlab/akita/v5/sim/modeling"
	"github.com/stretchr/testify/require"
)

// The serial baseline was captured from the hand-built program at 1d8104ba,
// seed 42, 100 reads and writes, 65536-byte address range: 2601000 ps.
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
			s.Add("", blueprint.NewDomain[platformSpec, modeling.None](platformSpec{MemoryCapacity: 4 * 1024 * 1024 * 1024},
				declarePlatform))
			require.NoError(t, s.Initialize())
			work := workload.DefaultSpec()
			work.Seed, work.ReadCount, work.WriteCount, work.AddressRange = 42, 100, 100, 65536
			workload.New(work).Execute(s)
			a := s.Lookup("MemAccessAgent").(*memaccessagent.Comp)
			require.NoError(t, s.Run())
			require.Zero(t, a.State.ReadLeft)
			require.Zero(t, a.State.WriteLeft)
			require.Empty(t, a.State.PendingReadReq)
			require.Empty(t, a.State.PendingWriteReq)
			if !parallel {
				require.EqualValues(t, 2601000, s.Engine().CurrentTime())
			}
			t.Logf("writeback %s: 100 reads and 100 writes complete at %d ps", name, s.Engine().CurrentTime())
		})
	}
}
