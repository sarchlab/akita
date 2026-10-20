package networkconnector

import (
	"testing"

	"github.com/sarchlab/akita/v5/noc/routing"
	"github.com/sarchlab/akita/v5/sim"
	"github.com/stretchr/testify/require"
)

func TestNamedSwitchAndExplicitPortQueues(t *testing.T) {
	s := sim.MakeBuilder().Build()
	table := routing.NewTable()
	c := MakeBuilder().WithSimulation(s).Build("Network")
	a := c.AddSwitch(SwitchConfig{Name: "Named", RoutingTable: table})
	b := c.AddSwitch(SwitchConfig{})
	_, sp := topologyParameters()
	sp.LeftEndParam.NumInputChannel = 3
	sp.LeftEndParam.NumOutputChannel = 2
	sp.LeftEndParam.IncomingBufSize = 7
	sp.LeftEndParam.OutgoingBufSize = 8
	sp.LeftEndParam.RouteBufferCapacity = 9
	sp.LeftEndParam.ForwardBufferCapacity = 10
	sp.LeftEndParam.SendBufferCapacity = 11
	c.ConnectSwitches(a, b, sp)
	require.NoError(t, s.Initialize())
	require.Same(t, table, c.switches[a].table)
	require.Equal(t, "Network.Named", c.switches[a].Name())
	require.Equal(t, "Network.Switch[1]", c.switches[b].Name())
	port := c.switches[a].ports[0]
	require.Equal(t, 7, port.IncomingBuf().Capacity())
	require.Equal(t, 8, port.OutgoingBuf().Capacity())
	state := &c.switches[a].sw.State.PortComplexes[0]
	require.Equal(t, 9, state.RouteBuffer.Capacity())
	require.Equal(t, 10, state.ForwardBuffer.Capacity())
	require.Equal(t, 11, state.SendOutBuffer.Capacity())
	require.Equal(t, 3, state.NumInputChannel)
	require.Equal(t, 2, state.NumOutputChannel)
}
