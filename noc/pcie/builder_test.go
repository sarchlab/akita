package pcie

import (
	"testing"

	"github.com/sarchlab/akita/v5/sim"
	"github.com/sarchlab/akita/v5/sim/timing"
	"github.com/stretchr/testify/require"
)

func TestBuilderOrderAndIndependentNetworks(t *testing.T) {
	b := MakeBuilder().WithSimulation(sim.MakeBuilder().Build())
	a := b.WithFlitSize(32).WithFreq(2 * timing.GHz).Build("A")
	c := b.WithFreq(2 * timing.GHz).WithFlitSize(32).Build("C")
	require.Equal(t, 32, a.flitByteSize)
	require.Equal(t, a.flitByteSize, c.flitByteSize)
	require.Equal(t, 0, a.AddRootComplex(nil))
	require.Equal(t, 1, a.AddSwitch(0))
	require.Equal(t, 0, c.AddRootComplex(nil))
	require.Equal(t, timing.GHz, b.freq)
}
