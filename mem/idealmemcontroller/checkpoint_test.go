package idealmemcontroller_test

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/sarchlab/akita/v5/mem"
	"github.com/sarchlab/akita/v5/mem/idealmemcontroller"
	"github.com/sarchlab/akita/v5/sim"
	"github.com/sarchlab/akita/v5/sim/messaging/twowaybuffered"
	"github.com/sarchlab/akita/v5/sim/modeling"
)

// TestCheckpointRoundTrip checkpoints a simulation containing a real memory
// controller — which registers Top and Control ports plus a storage resource —
// to confirm that a component with ports round-trips at a quiescent boundary.
// No traffic is driven, so the ports stay empty and the engine queue is idle.
func TestCheckpointRoundTrip(t *testing.T) {
	path := filepath.Join(t.TempDir(), "ck.tar.gz")
	const payload = "persisted bytes"

	s := sim.MakeBuilder().Build()
	defer func() {
		s.Terminate()
		os.Remove("akita_sim_" + s.ID() + ".sqlite3")
	}()

	// The storage registers itself with the simulation, so the checkpoint
	// includes its data.
	storage := mem.MakeStorageBuilder().
		WithCapacity(4 * mem.KB).
		WithSimulation(s).
		Build("DRAM.Storage")

	dram := modeling.MakeBuilder[*idealmemcontroller.Comp](idealmemcontroller.DefaultSpec()).
		WithSimulation(s).
		WithResources(idealmemcontroller.Resources{Storage: storage}).
		Build("DRAM")

	dram.BindPort("Top", twowaybuffered.MakeBuilder().
		WithSimulation(s).
		WithSpec(twowaybuffered.Spec{IncomingCapacity: 16, OutgoingCapacity: 16}).
		Build())
	dram.BindPort("Control", twowaybuffered.MakeBuilder().
		WithSimulation(s).
		WithSpec(twowaybuffered.Spec{IncomingCapacity: 16, OutgoingCapacity: 16}).
		Build())
	if err := s.Initialize(); err != nil {
		panic(err)
	}

	storage.Write(0x40, []byte(payload))
	dram.State.CurrentCmdID = 7

	if err := s.SaveCheckpoint(path); err != nil {
		t.Fatalf("SaveCheckpoint: %v", err)
	}

	// Mutate the resource and the component state away from the checkpoint.
	storage.Write(0x40, make([]byte, len(payload)))
	dram.State.CurrentCmdID = 99

	if err := s.LoadCheckpoint(path); err != nil {
		t.Fatalf("LoadCheckpoint: %v", err)
	}

	got := storage.Read(0x40, uint64(len(payload)))
	if string(got) != payload {
		t.Fatalf("storage = %q, want %q", got, payload)
	}
	if dram.State.CurrentCmdID != 7 {
		t.Fatalf("CurrentCmdID = %d, want 7", dram.State.CurrentCmdID)
	}
}
