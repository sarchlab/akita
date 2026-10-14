package writethrough

import (
	"testing"

	akitasim "github.com/sarchlab/akita/v5/sim"

	"github.com/sarchlab/akita/v5/mem"
	"github.com/sarchlab/akita/v5/sim/messaging"
	"github.com/stretchr/testify/require"
)

// TestBuildMapsRemotePorts checks that Build turns Spec.AddressMapperType and
// Resources.RemotePorts into the mapper the pipeline routes through.
func TestBuildMapsRemotePorts(t *testing.T) {
	spec := Definition.DefaultSpec
	spec.AddressMapperType = "interleaved"
	spec.InterleavingSize = 4096

	setupSim1 := akitasim.MakeBuilder().Build()
	spec.RemotePorts = []messaging.RemotePort{"DRAM0", "DRAM1"}
	comp := Definition.Builder().
		WithSimulation(setupSim1).
		WithSpec(spec).
		WithResources(Resources{Storage: mem.NewStorage(spec.TotalByteSize)}).
		Build("Cache")
	compPorts := makePorts("Cache", 4)
	comp.BindPort("Top", compPorts.Top)
	comp.BindPort("Bottom", compPorts.Bottom)
	comp.BindPort("Control", compPorts.Control)
	if err := setupSim1.Initialize(); err != nil {
		panic(err)
	}

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
	require.PanicsWithValue(t, "writethrough: Resources.Storage is required", func() {
		setupSim2 := akitasim.MakeBuilder().Build()
		builtComponent := Definition.Builder().
			WithSimulation(setupSim2).
			Build("Cache")
		builtComponentPorts := makePorts("Cache", 4)
		builtComponent.BindPort("Top", builtComponentPorts.Top)
		builtComponent.BindPort("Bottom", builtComponentPorts.Bottom)
		builtComponent.BindPort("Control", builtComponentPorts.Control)
		if err := setupSim2.Initialize(); err != nil {
			panic(err)
		}
	})
}
