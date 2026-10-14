package switches_test

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/sarchlab/akita/v5/noc/switching/switches"
	"github.com/sarchlab/akita/v5/sim"
)

// TestSwitchArbCursorRoundTrip guards that the switch's round-robin arbitration
// cursor is checkpointed. It used to live on the middleware (not in State), so a
// resumed switch restarted arbitration from port 0 and diverged from an
// uninterrupted run; the cursor now lives in State and must round-trip.
func TestSwitchArbCursorRoundTrip(t *testing.T) {
	path := filepath.Join(t.TempDir(), "ck.tar.gz")

	s := sim.MakeBuilder().Build()
	defer func() {
		s.Terminate()
		os.Remove("akita_sim_" + s.ID() + ".sqlite3")
	}()

	sw := switches.Definition.Builder().
		WithSimulation(s).
		Build("Switch")
	if err := s.Initialize(); err != nil {
		panic(err)
	}

	sw.State.NextArbPort = 2

	if err := s.SaveCheckpoint(path); err != nil {
		t.Fatalf("SaveCheckpoint: %v", err)
	}

	sw.State.NextArbPort = 99 // mutate away from the checkpoint

	if err := s.LoadCheckpoint(path); err != nil {
		t.Fatalf("LoadCheckpoint: %v", err)
	}
	if sw.State.NextArbPort != 2 {
		t.Fatalf("NextArbPort = %d, want 2 (arbitration cursor must round-trip)",
			sw.State.NextArbPort)
	}
}
