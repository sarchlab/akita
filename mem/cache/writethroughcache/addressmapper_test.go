package writethroughcache

import (
	"testing"

	"github.com/sarchlab/akita/v5/mem"
	"github.com/sarchlab/akita/v5/simulation/messaging"
	"github.com/sarchlab/akita/v5/simulation/modeling"
	"github.com/sarchlab/akita/v5/simulation/timing"
	"github.com/stretchr/testify/require"
)

// TestBuildMapsRemotePorts checks that Build turns Spec.AddressMapperType and
// Resources.RemotePorts into the mapper the pipeline routes through.
func TestBuildMapsRemotePorts(t *testing.T) {
	spec := Definition.DefaultSpec
	spec.AddressMapperType = "interleaved"
	spec.InterleavingSize = 4096

	comp := Definition.Builder().
		WithSimulation(modeling.NewStandaloneSimulation(timing.NewSerialEngine())).
		WithSpec(spec).
		WithResources(Resources{
			Storage:     mem.NewStorage(spec.TotalByteSize),
			RemotePorts: []messaging.RemotePort{"DRAM0", "DRAM1"},
		}).
		WithPorts(makePorts("Cache", 4)).
		Build("Cache")

	pipeline := comp.Middlewares.Pipeline
	want := map[uint64]messaging.RemotePort{0: "DRAM0", 4096: "DRAM1", 8192: "DRAM0"}
	for addr, port := range want {
		if got := pipeline.findPort(addr); got != port {
			t.Errorf("findPort(%d) = %s, want %s", addr, got, port)
		}
	}
}

// TestBuildRequiresStorage checks that Build rejects a cache without a
// backing storage instead of failing on the first bank access.
func TestBuildRequiresStorage(t *testing.T) {
	require.PanicsWithValue(t, "writethroughcache: Resources.Storage is required", func() {
		Definition.Builder().
			WithSimulation(modeling.NewStandaloneSimulation(timing.NewSerialEngine())).
			WithPorts(makePorts("Cache", 4)).
			Build("Cache")
	})
}
