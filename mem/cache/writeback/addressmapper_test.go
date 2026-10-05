package writeback

import (
	"bytes"
	"fmt"
	"strings"
	"testing"

	"github.com/sarchlab/akita/v5/mem"
	"github.com/sarchlab/akita/v5/messaging"
	"github.com/sarchlab/akita/v5/modeling"
	"github.com/sarchlab/akita/v5/timing"
)

// buildWired builds a cache named "Cache" with the given spec and resources.
// It supplies a storage when res has none.
func buildWired(res Resources, spec Spec) *Comp {
	sim := modeling.NewStandaloneSimulation(timing.NewSerialEngine())

	if res.Storage == nil {
		res.Storage = mem.NewStorage(spec.TotalByteSize)
	}

	return Definition.Builder().
		WithSimulation(sim).
		WithSpec(spec).
		WithResources(res).
		WithPorts(makePorts("Cache", 4)).
		Build("Cache")
}

// TestBuildMapsRemotePorts checks that Build turns Spec.AddressMapperType and
// Resources.RemotePorts into the mapper the pipeline routes through.
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
			pipeline := buildWired(Resources{RemotePorts: ports}, c.spec).
				Middlewares.Pipeline
			for addr, want := range c.want {
				if got := pipeline.findPort(addr); got != want {
					t.Errorf("findPort(%d) = %s, want %s", addr, got, want)
				}
			}
		})
	}
}

// TestRestoreRoutesThroughRebuiltWiring saves a cache wired to one lower
// module and loads the checkpoint into a cache rebuilt against another.
// Wiring comes from Resources, which the rebuild supplies, so the restored
// cache must route to the new module rather than to the checkpointed one.
func TestRestoreRoutesThroughRebuiltWiring(t *testing.T) {
	spec := Definition.DefaultSpec

	saved := buildWired(Resources{
		AddressToPortMapper: &mem.SinglePortMapper{Port: "OldMemory.Top"},
	}, spec)

	var checkpoint bytes.Buffer
	if err := saved.SaveCheckpoint(&checkpoint); err != nil {
		t.Fatalf("SaveCheckpoint: %v", err)
	}

	rebuilt := buildWired(Resources{
		AddressToPortMapper: &mem.SinglePortMapper{Port: "NewMemory.Top"},
	}, spec)
	if err := rebuilt.LoadCheckpoint(&checkpoint); err != nil {
		t.Fatalf("LoadCheckpoint: %v", err)
	}

	pipeline := rebuilt.Middlewares.Pipeline
	if got := pipeline.findPort(0); got != "NewMemory.Top" {
		t.Errorf("restored cache routes to %s, want NewMemory.Top", got)
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
		buildWired(Resources{RemotePorts: []messaging.RemotePort{"DRAM0"}}, spec)
	})
}

// TestBuildRequiresStorage checks that Build rejects a cache without a
// backing storage instead of failing on the first bank access.
func TestBuildRequiresStorage(t *testing.T) {
	assertPanics(t, "Resources.Storage is required", func() {
		Definition.Builder().
			WithSimulation(modeling.NewStandaloneSimulation(timing.NewSerialEngine())).
			WithPorts(makePorts("Cache", 4)).
			Build("Cache")
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
