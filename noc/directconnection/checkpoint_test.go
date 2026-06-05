package directconnection_test

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/sarchlab/akita/v5/noc/directconnection"
	"github.com/sarchlab/akita/v5/simulation"
)

// TestDirectConnectionCursorRoundTrip confirms a registered connection is part
// of the checkpoint inventory and that its round-robin cursor round-trips. The
// connection is registered via RegisterConnection (not RegisterComponent), so
// this also guards that connections reach the entity inventory.
func TestDirectConnectionCursorRoundTrip(t *testing.T) {
	path := filepath.Join(t.TempDir(), "ck.tar.gz")
	const buildID = "conn-test"

	sim := simulation.MakeBuilder().WithoutMonitoring().Build()
	defer func() {
		sim.Terminate()
		os.Remove("akita_sim_" + sim.ID() + ".sqlite3")
	}()

	conn := directconnection.MakeBuilder().WithRegistrar(sim).Build("Conn")
	conn.State.NextPortID = 3

	if err := sim.SaveCheckpoint(path, buildID); err != nil {
		t.Fatalf("SaveCheckpoint: %v", err)
	}

	conn.State.NextPortID = 99 // mutate away from the checkpoint

	if err := sim.LoadCheckpoint(path, buildID); err != nil {
		t.Fatalf("LoadCheckpoint: %v", err)
	}
	if conn.State.NextPortID != 3 {
		t.Fatalf("NextPortID = %d, want 3", conn.State.NextPortID)
	}
}
