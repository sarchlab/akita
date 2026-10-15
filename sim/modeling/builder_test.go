package modeling_test

import (
	"testing"

	"github.com/sarchlab/akita/v5/mem"
	"github.com/sarchlab/akita/v5/mem/idealmemcontroller"
	"github.com/sarchlab/akita/v5/sim"
	"github.com/sarchlab/akita/v5/sim/modeling"
	"github.com/stretchr/testify/require"
)

func TestBuilderSuppliesExternalResources(t *testing.T) {
	simulation := sim.MakeBuilder().Build()
	t.Cleanup(simulation.Terminate)
	storage := mem.NewStorage(64)
	spec := idealmemcontroller.DefaultSpec()
	spec.Latency = 7
	builder := modeling.MakeBuilder[*idealmemcontroller.Comp](spec).
		WithSimulation(simulation).
		WithResources(idealmemcontroller.Resources{Storage: storage})
	a, b := builder.Build("A"), builder.Build("B")
	spec.Latency = 99
	require.Equal(t, 7, a.Spec.Latency)
	require.Equal(t, 7, b.Spec.Latency)
	require.Same(t, storage, a.Resources.Storage)
	require.Same(t, a.Resources.Storage, b.Resources.Storage)
	require.NotSame(t, a, b)
	require.NotSame(t, &a.State, &b.State)
	t.Log("Typed builders create independent components with Latency=7 and the same externally supplied Storage")
}
