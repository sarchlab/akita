package datamover

import (
	"bytes"
	"testing"

	"github.com/sarchlab/akita/v5/mem"
	"github.com/sarchlab/akita/v5/messaging"
	"github.com/sarchlab/akita/v5/modeling"
	"github.com/sarchlab/akita/v5/timing"
)

func buildWithMappers(inside, outside messaging.RemotePort) *Comp {
	return Definition.Builder().
		WithSimulation(modeling.NewStandaloneSimulation(timing.NewSerialEngine())).
		WithSpec(Definition.DefaultSpec).
		WithResources(Resources{
			InsideMapper:  &mem.SinglePortMapper{Port: inside},
			OutsideMapper: &mem.SinglePortMapper{Port: outside},
		}).
		WithPorts(makePorts("DataMover", 1, 1, 1, 1)).
		Build("DataMover")
}

func dataTransferOf(t *testing.T, comp *Comp) *dataTransferMW {
	t.Helper()

	if comp.Middlewares.DataTransfer == nil {
		t.Fatal("Build installed no dataTransferMW")
	}

	return comp.Middlewares.DataTransfer
}

// TestRestoreRoutesThroughRebuiltMappers saves a data mover wired to one pair
// of memories and loads the checkpoint into one rebuilt against another pair.
// The mappers come from Resources, which the rebuild supplies, so the
// restored data mover must route through the new ones.
func TestRestoreRoutesThroughRebuiltMappers(t *testing.T) {
	saved := buildWithMappers("OldInside.Top", "OldOutside.Top")

	var checkpoint bytes.Buffer
	if err := saved.SaveCheckpoint(&checkpoint); err != nil {
		t.Fatalf("SaveCheckpoint: %v", err)
	}

	rebuilt := buildWithMappers("NewInside.Top", "NewOutside.Top")
	if err := rebuilt.LoadCheckpoint(&checkpoint); err != nil {
		t.Fatalf("LoadCheckpoint: %v", err)
	}

	rebuilt.State.SrcSide = "inside"
	rebuilt.State.DstSide = "outside"
	dt := dataTransferOf(t, rebuilt)

	if got := dt.findSrcPort(0); got != "NewInside.Top" {
		t.Errorf("source routes to %s, want NewInside.Top", got)
	}

	if got := dt.findDstPort(0); got != "NewOutside.Top" {
		t.Errorf("destination routes to %s, want NewOutside.Top", got)
	}
}
