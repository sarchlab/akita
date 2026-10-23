package writethrough

import (
	"testing"

	"github.com/sarchlab/akita/v5/mem"
	akitasim "github.com/sarchlab/akita/v5/sim"
	"github.com/sarchlab/akita/v5/sim/messaging"
	"github.com/sarchlab/akita/v5/sim/modeling"
	"github.com/stretchr/testify/require"
)

// TestBuildMapsRemotePorts checks routing directly through Spec.Bottom.
func TestBuildMapsRemotePorts(t *testing.T) {
	spec := DefaultSpec()

	spec.Bottom.InterleavingSize = 4096

	setupSim1 := akitasim.MakeBuilder().Build()
	spec.Bottom.Destinations = []messaging.RemotePort{"DRAM0", "DRAM1"}
	comp := modeling.MakeBuilder[*Comp](spec).
		WithSimulation(setupSim1).
		WithResources(Resources{Storage: mem.NewStorage(spec.ByteSize())}).
		Build("Cache")
	compPorts := makePorts(setupSim1, "Cache", 4)
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
		spec := DefaultSpec()
		spec.Bottom.Destinations = []messaging.RemotePort{"DRAM"}
		builtComponent := modeling.MakeBuilder[*Comp](spec).
			WithSimulation(setupSim2).
			Build("Cache")
		builtComponentPorts := makePorts(setupSim2, "Cache", 4)
		builtComponent.BindPort("Top", builtComponentPorts.Top)
		builtComponent.BindPort("Bottom", builtComponentPorts.Bottom)
		builtComponent.BindPort("Control", builtComponentPorts.Control)
		if err := setupSim2.Initialize(); err != nil {
			panic(err)
		}
	})
}
