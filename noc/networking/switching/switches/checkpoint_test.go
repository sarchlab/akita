package switches_test

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/sarchlab/akita/v5/noc/networking/routing"
	"github.com/sarchlab/akita/v5/noc/networking/switching/switches"
	"github.com/sarchlab/akita/v5/simulation"
)

// TestSwitchArbCursorRoundTrip guards that the switch's round-robin arbitration
// cursor is checkpointed. It used to live on the middleware (not in State), so a
// resumed switch restarted arbitration from port 0 and diverged from an
// uninterrupted run; the cursor now lives in State and must round-trip.
func TestSwitchArbCursorRoundTrip(t *testing.T) {
	path := filepath.Join(t.TempDir(), "ck.tar.gz")
	const buildID = "switch-test"

	sim := simulation.MakeBuilder().WithoutMonitoring().Build()
	defer func() {
		sim.Terminate()
		os.Remove("akita_sim_" + sim.ID() + ".sqlite3")
	}()

	sw := switches.MakeBuilder().
		WithRegistrar(sim).
		WithResources(switches.Resources{RoutingTable: routing.NewTable()}).
		Build("Switch")
	sw.State.NextArbPort = 2

	if err := sim.SaveCheckpoint(path, buildID); err != nil {
		t.Fatalf("SaveCheckpoint: %v", err)
	}

	sw.State.NextArbPort = 99 // mutate away from the checkpoint

	if err := sim.LoadCheckpoint(path, buildID); err != nil {
		t.Fatalf("LoadCheckpoint: %v", err)
	}
	if sw.State.NextArbPort != 2 {
		t.Fatalf("NextArbPort = %d, want 2 (arbitration cursor must round-trip)",
			sw.State.NextArbPort)
	}
}
