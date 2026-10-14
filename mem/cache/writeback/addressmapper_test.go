package writeback

import (
	"bytes"
	"fmt"
	"strings"
	"testing"

	akitasim "github.com/sarchlab/akita/v5/sim"

	"github.com/sarchlab/akita/v5/mem"
	"github.com/sarchlab/akita/v5/sim/messaging"
)

// buildWired builds a cache named "Cache" with the given spec and resources.
// It supplies a storage when res has none.
func buildWired(res Resources, spec Spec) *Comp {
	sim := akitasim.MakeBuilder().Build()

	if res.Storage == nil {
		res.Storage = mem.NewStorage(spec.TotalByteSize)
	}

	builtComponent := Definition.Builder().
		WithSimulation(sim).
		WithSpec(spec).
		WithResources(res).
		Build("Cache")
	builtComponentPorts := makePorts("Cache", 4)
	builtComponent.BindPort("Top", builtComponentPorts.Top)
	builtComponent.BindPort("Bottom", builtComponentPorts.Bottom)
	builtComponent.BindPort("Control", builtComponentPorts.Control)
	if err := sim.Initialize(); err != nil {
		panic(err)
	}

	return builtComponent
}

// TestBuildMapsRemotePorts checks that Build turns Spec.AddressMapperType and
// Spec.RemotePorts into the mapper the pipeline routes through.
func TestBuildMapsRemotePorts(t *testing.T) {
	ports := []messaging.RemotePort{"DRAM0", "DRAM1"}

	single := Definition.DefaultSpec
	single.AddressMapperType = "single"

	interleaved := Definition.DefaultSpec
	interleaved.AddressMapperType = "interleaved"
	interleaved.InterleavingSize = 4096

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
			c.spec.RemotePorts = ports
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
	spec := Definition.DefaultSpec
	spec.RemotePorts = []messaging.RemotePort{"OldMemory.Top"}
	saved := buildWired(Resources{}, spec)
	var checkpoint bytes.Buffer
	if err := saved.SaveCheckpoint(&checkpoint); err != nil {
		t.Fatal(err)
	}
	original := append([]byte(nil), checkpoint.Bytes()...)
	spec.RemotePorts = []messaging.RemotePort{"NewMemory.Top"}
	rebuilt := buildWired(Resources{}, spec)
	if err := rebuilt.LoadCheckpoint(&checkpoint); err == nil {
		t.Fatal("accepted a different routing Spec")
	}
	spec.RemotePorts = []messaging.RemotePort{"OldMemory.Top"}
	same := buildWired(Resources{}, spec)
	if err := same.LoadCheckpoint(bytes.NewReader(original)); err != nil {
		t.Fatal(err)
	}
	if got := same.Middlewares.Pipeline.findPort(0); got != "OldMemory.Top" {
		t.Fatalf("restored destination = %s", got)
	}
}

// TestBuildRejectsIncompleteInterleaving checks the two ways an interleaved
// mapper can be misconfigured: no remote ports defers to findPort's clear
// panic, and a zero interleaving size fails at Build instead of dividing by
// zero on the first miss.
func TestBuildRejectsIncompleteInterleaving(t *testing.T) {
	spec := Definition.DefaultSpec
	spec.AddressMapperType = "interleaved"
	spec.InterleavingSize = 4096

	noPorts := buildWired(Resources{}, spec).Middlewares.Pipeline
	if mapper := noPorts.addressMapper; mapper != nil {
		t.Errorf("mapper without remote ports = %v, want nil", mapper)
	}

	assertPanics(t, "no address mapper", func() {
		noPorts.findPort(0)
	})

	spec.InterleavingSize = 0
	assertPanics(t, "non-zero Spec.InterleavingSize", func() {
		spec.RemotePorts = []messaging.RemotePort{"DRAM0"}
		buildWired(Resources{}, spec)
	})
}

// TestBuildRequiresStorage checks that Build rejects a cache without a
// backing storage instead of failing on the first bank access.
func TestBuildRequiresStorage(t *testing.T) {
	assertPanics(t, "Resources.Storage is required", func() {
		setupSim1 := akitasim.MakeBuilder().Build()
		builtComponent := Definition.Builder().
			WithSimulation(setupSim1).
			Build("Cache")
		builtComponentPorts := makePorts("Cache", 4)
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
