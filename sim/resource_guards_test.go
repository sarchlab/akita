package sim_test

import (
	"testing"

	"github.com/sarchlab/akita/v5/mem"
	"github.com/sarchlab/akita/v5/mem/vm"
	"github.com/sarchlab/akita/v5/sim"
	"github.com/sarchlab/akita/v5/sim/messaging/direct"
	"github.com/sarchlab/akita/v5/sim/timing"
	"github.com/stretchr/testify/require"
)

func TestResourceBuildersRequireSimulationAndRegister(t *testing.T) {
	require.PanicsWithValue(t, "Storage: WithSimulation is required",
		func() { mem.MakeStorageBuilder().Build("Storage") })
	require.PanicsWithValue(t, "PageTable: WithSimulation is required",
		func() { vm.MakePageTableBuilder().Build("Pages") })
	s := sim.MakeBuilder().Build()
	mem.MakeStorageBuilder().WithSimulation(s).Build("Storage")
	vm.MakePageTableBuilder().WithSimulation(s).Build("Pages")
	require.Panics(t, func() { vm.MakePageTableBuilder().WithSimulation(s).Build("Storage") })
	require.Panics(t, func() { mem.MakeStorageBuilder().WithSimulation(s).Build("Pages") })
	require.Panics(t, func() { mem.MakeStorageBuilder().WithSimulation(s).Build("") })
	require.NoError(t, s.Initialize())
	require.NoError(t, s.Terminate())
}

func TestZeroFrequencyConnectionDoesNotRegister(t *testing.T) {
	s := sim.MakeBuilder().Build()
	require.PanicsWithValue(t, `direct: connection "Link" requires a nonzero frequency`,
		func() { direct.NewConnection("Link", s, 0) })
	require.NotPanics(t, func() { direct.NewConnection("Link", s, timing.GHz) })
	require.NoError(t, s.Initialize())
	require.NoError(t, s.Terminate())
}
