package nvlink

import (
	"testing"

	"github.com/sarchlab/akita/v5/sim"
	"github.com/sarchlab/akita/v5/sim/timing"
	"github.com/stretchr/testify/require"
)

func TestBuilderOrderAndIndependentNetworks(t *testing.T) {
	b := MakeBuilder().WithSimulation(sim.MakeBuilder().Build())
	a := b.WithPCIeBandwidth(64e9).WithFreq(2 * timing.GHz).Build("A")
	c := b.WithFreq(2 * timing.GHz).WithPCIeBandwidth(64e9).Build("C")
	require.Equal(t, 32, a.flitByteSize)
	require.Equal(t, a.flitByteSize, c.flitByteSize)
	a.AddPCIeSwitch()
	require.Len(t, a.pcieSwitches, 1)
	require.Empty(t, c.pcieSwitches)
	require.Equal(t, 0, c.AddPCIeSwitch())
	require.Equal(t, timing.GHz, b.freq)
}
