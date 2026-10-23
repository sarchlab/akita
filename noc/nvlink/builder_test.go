package nvlink

import (
	"testing"

	"github.com/sarchlab/akita/v5/noc/internal/endpoint"
	"github.com/sarchlab/akita/v5/sim"
	"github.com/sarchlab/akita/v5/sim/timing"
	"github.com/stretchr/testify/require"
)

func TestBuilderOrderAndIndependentNetworks(t *testing.T) {
	s := sim.MakeBuilder().Build()
	b := MakeBuilder().WithSimulation(s)
	a := b.WithFlitSize(32).WithFreq(2 * timing.GHz).Build("A")
	c := b.WithFreq(2 * timing.GHz).WithFlitSize(32).Build("C")
	a.ConnectDevice(a.AddSwitch(), nil)
	require.Len(t, a.pcieSwitches, 1)
	require.Empty(t, c.pcieSwitches)
	require.Equal(t, 0, c.AddSwitch())
	c.ConnectDevice(0, nil)
	require.Equal(t, timing.GHz, b.freq)
	require.NoError(t, s.Initialize())
	endpoints := 0
	for _, comp := range s.Components() {
		if ep, ok := comp.(*endpoint.Comp); ok {
			endpoints++
			require.Equal(t, 32, ep.Spec.FlitByteSize)
			require.Equal(t, 2*timing.GHz, ep.Spec.Freq)
		}
	}
	require.Equal(t, 2, endpoints)
}
