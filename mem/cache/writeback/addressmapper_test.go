package writeback

import (
	"bytes"
	"testing"

	"github.com/sarchlab/akita/v5/mem"
	"github.com/sarchlab/akita/v5/messaging"
	"github.com/sarchlab/akita/v5/modeling"
	"github.com/sarchlab/akita/v5/timing"
)

func buildWired(res Resources, spec Spec) *Comp {
	sim := modeling.NewStandaloneSimulation(timing.NewSerialEngine())

	return MakeBuilder().
		WithSimulation(sim).
		WithSpec(spec).
		WithResources(res).
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
			pipeline := &pipelineMW{comp: buildWired(Resources{RemotePorts: ports}, c.spec)}
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

	pipeline := &pipelineMW{comp: rebuilt}
	if got := pipeline.findPort(0); got != "NewMemory.Top" {
		t.Errorf("restored cache routes to %s, want NewMemory.Top", got)
	}
}
