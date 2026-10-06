package switches_test

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/sarchlab/akita/v5/noc/networking/routing"
	"github.com/sarchlab/akita/v5/noc/networking/switching/switches"
	"github.com/sarchlab/akita/v5/sim"
)

// TestSwitchArbCursorRoundTrip guards that the switch's round-robin arbitration
// cursor is checkpointed. It used to live on the middleware (not in State), so a
// resumed switch restarted arbitration from port 0 and diverged from an
// uninterrupted run; the cursor now lives in State and must round-trip.
func TestSwitchArbCursorRoundTrip(t *testing.T) {
	path := filepath.Join(t.TempDir(), "ck.tar.gz")
	const buildID = "switch-test"

	s := sim.MakeBuilder().Build()
	defer func() {
		s.Terminate()
		os.Remove("akita_sim_" + s.ID() + ".sqlite3")
	}()

	sw := switches.Definition.Builder().
		WithSimulation(s).
		WithResources(switches.Resources{RoutingTable: routing.NewTable()}).
		Build("Switch")
	sw.State.NextArbPort = 2

	if err := s.SaveCheckpoint(path, buildID); err != nil {
		t.Fatalf("SaveCheckpoint: %v", err)
	}

	sw.State.NextArbPort = 99 // mutate away from the checkpoint

	if err := s.LoadCheckpoint(path, buildID); err != nil {
		t.Fatalf("LoadCheckpoint: %v", err)
	}
	if sw.State.NextArbPort != 2 {
		t.Fatalf("NextArbPort = %d, want 2 (arbitration cursor must round-trip)",
			sw.State.NextArbPort)
	}
}
