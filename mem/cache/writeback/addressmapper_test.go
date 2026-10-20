package writeback

import (
	"bytes"
	"fmt"
	"strings"
	"testing"

	"github.com/sarchlab/akita/v5/mem"
	akitasim "github.com/sarchlab/akita/v5/sim"
	"github.com/sarchlab/akita/v5/sim/messaging"
	"github.com/sarchlab/akita/v5/sim/modeling"
)

// buildWired builds a cache named "Cache" with the given spec and resources.
// It supplies a storage when res has none.
func buildWired(res Resources, spec Spec) *Comp {
	sim := akitasim.MakeBuilder().Build()

	if res.Storage == nil {
		res.Storage = mem.NewStorage(spec.ByteSize())
	}

	builtComponent := modeling.MakeBuilder[*Comp](spec).
		WithSimulation(sim).
		WithResources(res).
		Build("Cache")
	builtComponentPorts := makePorts(sim, "Cache", 4)
	builtComponent.BindPort("Top", builtComponentPorts.Top)
	builtComponent.BindPort("Bottom", builtComponentPorts.Bottom)
	builtComponent.BindPort("Control", builtComponentPorts.Control)
	if err := sim.Initialize(); err != nil {
		panic(err)
	}

	return builtComponent
}

// TestBuildMapsRemotePorts checks routing directly through Spec.Bottom.
func TestBuildMapsRemotePorts(t *testing.T) {
	ports := []messaging.RemotePort{"DRAM0", "DRAM1"}

	single := DefaultSpec()
	single.Bottom.Destinations = ports[:1]

	interleaved := DefaultSpec()
	interleaved.Bottom.Destinations = ports

	interleaved.Bottom.InterleavingSize = 4096

	cases := []struct {
		name string
		spec Spec
		want map[uint64]messaging.RemotePort
	}{
		{"single", single, map[uint64]messaging.RemotePort{0: "DRAM0", 8192: "DRAM0"}},
		{"interleaved", interleaved, map[uint64]messaging.RemotePort{
			0: "DRAM0", 4096: "DRAM1", 8192: "DRAM0"}},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			pipeline := buildWired(Resources{}, c.spec).
				Middlewares.Pipeline
			for addr, want := range c.want {
				if got := pipeline.findPort(addr); got != want {
					t.Errorf("findPort(%d) = %s, want %s", addr, got, want)
				}
			}
		})
	}
}

// TestRestoreRejectsChangedRouting confirms destinations participate in the Spec hash.
func TestRestoreRejectsChangedRouting(t *testing.T) {
	spec := DefaultSpec()
	spec.Bottom.Destinations = []messaging.RemotePort{"OldMemory.Top"}
	saved := buildWired(Resources{}, spec)
	var checkpoint bytes.Buffer
	if err := saved.SaveCheckpoint(&checkpoint); err != nil {
		t.Fatal(err)
	}
	original := append([]byte(nil), checkpoint.Bytes()...)
	spec.Bottom.Destinations = []messaging.RemotePort{"NewMemory.Top"}
	rebuilt := buildWired(Resources{}, spec)
	if err := rebuilt.LoadCheckpoint(&checkpoint); err == nil {
		t.Fatal("accepted a different routing Spec")
	}
	spec.Bottom.Destinations = []messaging.RemotePort{"OldMemory.Top"}
	same := buildWired(Resources{}, spec)
	if err := same.LoadCheckpoint(bytes.NewReader(original)); err != nil {
		t.Fatal(err)
	}
	if got := same.Middlewares.Pipeline.findPort(0); got != "OldMemory.Top" {
		t.Fatalf("restored destination = %s", got)
	}
}

// TestInitializationRejectsIncompleteInterleaving checks invalid routing during initialization.
func TestInitializationRejectsIncompleteInterleaving(t *testing.T) {
	spec := DefaultSpec()

	spec.Bottom.InterleavingSize = 4096

	assertPanics(t, "Cache: address mapper: at least one destination", func() { buildWired(Resources{}, spec) })
	spec.Bottom.InterleavingSize = 0
	spec.Bottom.Destinations = []messaging.RemotePort{"DRAM0", "DRAM1"}
	assertPanics(t, "positive InterleavingSize", func() { buildWired(Resources{}, spec) })
}

// TestBuildRequiresStorage checks that Build rejects a cache without a
// backing storage instead of failing on the first bank access.
func TestBuildRequiresStorage(t *testing.T) {
	assertPanics(t, "Resources.Storage is required", func() {
		setupSim1 := akitasim.MakeBuilder().Build()
		spec := DefaultSpec()
		spec.Bottom.Destinations = []messaging.RemotePort{"DRAM"}
		builtComponent := modeling.MakeBuilder[*Comp](spec).
			WithSimulation(setupSim1).
			Build("Cache")
		builtComponentPorts := makePorts(setupSim1, "Cache", 4)
		builtComponent.BindPort("Top", builtComponentPorts.Top)
		builtComponent.BindPort("Bottom", builtComponentPorts.Bottom)
		builtComponent.BindPort("Control", builtComponentPorts.Control)
		if err := setupSim1.Initialize(); err != nil {
			panic(err)
		}
	})
}

func assertPanics(t *testing.T, substr string, f func()) {
	t.Helper()

	defer func() {
		t.Helper()

		r := recover()
		if r == nil || !strings.Contains(fmt.Sprint(r), substr) {
			t.Fatalf("panic = %v, want one containing %q", r, substr)
		}
	}()

	f()
}
