package datamover

import (
	"bytes"
	"testing"

	akitasim "github.com/sarchlab/akita/v5/sim"

	"github.com/sarchlab/akita/v5/sim/messaging"
)

func buildWithMappers(inside, outside messaging.RemotePort) *Comp {
	setupSim1 := akitasim.MakeBuilder().Build()
	routingSpec1 := Definition.DefaultSpec

	routingSpec1.Inside.Destinations = []messaging.RemotePort{inside}

	routingSpec1.Outside.Destinations = []messaging.RemotePort{outside}
	builtComponent := Definition.Builder().
		WithSimulation(setupSim1).
		WithSpec(routingSpec1).
		Build("DataMover")
	builtComponentPorts := makePorts("DataMover", 1, 1, 1, 1)
	builtComponent.BindPort("Top", builtComponentPorts.Top)
	builtComponent.BindPort("Inside", builtComponentPorts.Inside)
	builtComponent.BindPort("Outside", builtComponentPorts.Outside)
	builtComponent.BindPort("Control", builtComponentPorts.Control)
	if err := setupSim1.Initialize(); err != nil {
		panic(err)
	}

	return builtComponent
}

func dataTransferOf(t *testing.T, comp *Comp) *dataTransferMW {
	t.Helper()

	if comp.Middlewares.DataTransfer == nil {
		t.Fatal("Build installed no dataTransferMW")
	}

	return comp.Middlewares.DataTransfer
}

// TestRestoreRoutingSpec rejects changed destinations and restores identical wiring.
func TestRestoreRoutingSpec(t *testing.T) {
	saved := buildWithMappers("Inside.Top", "Outside.Top")
	var checkpoint bytes.Buffer
	if err := saved.SaveCheckpoint(&checkpoint); err != nil {
		t.Fatal(err)
	}
	original := append([]byte(nil), checkpoint.Bytes()...)
	changed := buildWithMappers("OtherInside.Top", "Outside.Top")
	if err := changed.LoadCheckpoint(&checkpoint); err == nil {
		t.Fatal("accepted changed routing Spec")
	}
	rebuilt := buildWithMappers("Inside.Top", "Outside.Top")
	if err := rebuilt.LoadCheckpoint(bytes.NewReader(original)); err != nil {
		t.Fatal(err)
	}
	rebuilt.State.SrcSide = "inside"
	rebuilt.State.DstSide = "outside"
	dt := dataTransferOf(t, rebuilt)
	if got := dt.findSrcPort(0); got != "Inside.Top" {
		t.Fatalf("source routes to %s", got)
	}
	if got := dt.findDstPort(0); got != "Outside.Top" {
		t.Fatalf("destination routes to %s", got)
	}
}
