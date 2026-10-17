package endpoint

import (
	"testing"

	akitasim "github.com/sarchlab/akita/v5/sim"
	"github.com/sarchlab/akita/v5/sim/messaging"
	"github.com/sarchlab/akita/v5/sim/messaging/twowaybuffered"
	"github.com/stretchr/testify/require"
)

func buildWiring(t *testing.T, bound []messaging.RemotePort) (*akitasim.Simulation, *Comp) {
	t.Helper()
	s := akitasim.MakeBuilder().Build()
	spec := DefaultSpec()

	ep := MakeBuilder().WithSimulation(s).WithSpec(spec).Build("EP")
	ep.BindPort("NetworkPort", twowaybuffered.NewPort(1, 1))
	ep.Resources.Peer = connectPeer(s, ep.Ports.NetworkPort)
	for _, name := range bound {
		p := twowaybuffered.NewPort(1, 1)
		p.BindOwner(payloadOwner{string(name)}, string(name))
		DeviceConnection(ep).BindPort(p)
		s.RegisterPort(p)
	}
	return s, ep
}
func TestDeviceBindingOrderAndFreeze(t *testing.T) {
	s, ep := buildWiring(t, []messaging.RemotePort{"A", "B"})
	require.NoError(t, s.Initialize())
	require.Len(t, ep.Middlewares.Outgoing.devicePorts, 2)
	p := twowaybuffered.NewPort(1, 1)
	p.BindOwner(payloadOwner{"C"}, "C")
	require.Panics(t, func() { DeviceConnection(ep).BindPort(p) })
	require.Nil(t, p.Connection())
}
func TestBindingRejectsUnownedDuplicateAndConnectedPorts(t *testing.T) {
	s, ep := buildWiring(t, []messaging.RemotePort{"A"})
	require.Panics(t, func() { DeviceConnection(ep).BindPort(twowaybuffered.NewPort(1, 1)) })
	p := ep.Resources.devices.ports[0]
	require.Panics(t, func() { DeviceConnection(ep).BindPort(p) })
	other := MakeBuilder().WithSimulation(s).Build("Other")
	require.Panics(t, func() { DeviceConnection(other).BindPort(p) })
	require.Empty(t, other.Resources.devices.ports)
}
