package networkconnector

import (
	"testing"

	"github.com/sarchlab/akita/v5/noc/routing"
	"github.com/sarchlab/akita/v5/sim"
	"github.com/stretchr/testify/require"
)

func TestNamedSwitchAndCalculatedPortQueues(t *testing.T) {
	s := sim.MakeBuilder().Build()
	table := routing.NewTable()
	c := MakeBuilder().WithSimulation(s).Build("Network")
	a := c.AddSwitch(SwitchConfig{Name: "Named", RoutingTable: table})
	b := c.AddSwitch(SwitchConfig{})
	_, sp := topologyParameters()
	sp.LeftEndParam.InputWidth = 3
	sp.LeftEndParam.OutputWidth = 2

	c.ConnectSwitches(a, b, sp)
	require.NoError(t, s.Initialize())
	require.Same(t, table, c.switches[a].table)
	require.Equal(t, "Network.Named", c.switches[a].Name())
	require.Equal(t, "Network.Switch[1]", c.switches[b].Name())
	port := c.switches[a].ports[0]
	require.Equal(t, 3, port.IncomingBuf().Capacity())
	require.Equal(t, 2, port.OutgoingBuf().Capacity())
	state := &c.switches[a].sw.State.PortComplexes[0]
	require.Equal(t, 3, state.RouteBuffer.Capacity())
	require.Equal(t, 3, state.ForwardBuffer.Capacity())
	require.Equal(t, 2, state.SendOutBuffer.Capacity())
	require.Equal(t, 3, state.NumInputChannel)
	require.Equal(t, 2, state.NumOutputChannel)
}
