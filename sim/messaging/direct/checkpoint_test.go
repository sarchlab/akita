package direct_test

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/sarchlab/akita/v5/sim"
	"github.com/sarchlab/akita/v5/sim/messaging/direct"
	"github.com/sarchlab/akita/v5/sim/timing"
)

// TestDirectConnectionCursorRoundTrip confirms a registered connection is part
// of the checkpoint inventory and that its round-robin cursor round-trips. The
// connection is registered via RegisterConnection (not RegisterComponent), so
// this also guards that connections reach the entity inventory.
func TestDirectConnectionCursorRoundTrip(t *testing.T) {
	path := filepath.Join(t.TempDir(), "ck.tar.gz")
	const buildID = "conn-test"

	s := sim.MakeBuilder().Build()
	defer func() {
		s.Terminate()
		os.Remove("akita_sim_" + s.ID() + ".sqlite3")
	}()

	conn := direct.NewConnection("Conn", s, timing.GHz)
	conn.State.NextPortID = 3

	if err := s.SaveCheckpoint(path, buildID); err != nil {
		t.Fatalf("SaveCheckpoint: %v", err)
	}

	conn.State.NextPortID = 99 // mutate away from the checkpoint

	if err := s.LoadCheckpoint(path, buildID); err != nil {
		t.Fatalf("LoadCheckpoint: %v", err)
	}
	if conn.State.NextPortID != 3 {
		t.Fatalf("NextPortID = %d, want 3", conn.State.NextPortID)
	}
}
