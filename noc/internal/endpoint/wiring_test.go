package endpoint

import (
	"bytes"
	"testing"

	akitasim "github.com/sarchlab/akita/v5/sim"
	"github.com/sarchlab/akita/v5/sim/messaging"
	"github.com/sarchlab/akita/v5/sim/messaging/twowaybuffered"
	"github.com/stretchr/testify/require"
)

func buildWiring(t *testing.T, declared, bound []messaging.RemotePort) (*akitasim.Simulation, *Comp) {
	t.Helper()
	s := akitasim.MakeBuilder().Build()
	spec := Definition.DefaultSpec
	spec.Devices = declared
	ep := MakeBuilder().WithSimulation(s).WithSpec(spec).Build("EP")
	ep.BindPort("NetworkPort", twowaybuffered.NewPort(1, 1))
	for _, name := range bound {
		p := twowaybuffered.NewPort(1, 1)
		p.BindOwner(payloadOwner{string(name)}, string(name))
		DeviceConnection(ep).BindPort(p)
	}
	return s, ep
}
func TestDeviceBindingOrderAndFreeze(t *testing.T) {
	for _, bound := range [][]messaging.RemotePort{{"A"}, {"B", "A"}, {"A", "C"}} {
		s, _ := buildWiring(t, []messaging.RemotePort{"A", "B"}, bound)
		require.Panics(t, func() { _ = s.Initialize() })
		require.Error(t, s.Initialize())
		require.Error(t, s.Run())
	}
	s, ep := buildWiring(t, []messaging.RemotePort{"A", "B"}, []messaging.RemotePort{"A", "B"})
	require.NoError(t, s.Initialize())
	require.Len(t, ep.Middlewares.Outgoing.devicePorts, 2)
	p := twowaybuffered.NewPort(1, 1)
	p.BindOwner(payloadOwner{"C"}, "C")
	require.Panics(t, func() { DeviceConnection(ep).BindPort(p) })
	require.Nil(t, p.Connection())
}
func TestCheckpointRejectsChangedDeviceOrder(t *testing.T) {
	s, ep := buildWiring(t, []messaging.RemotePort{"A", "B"}, []messaging.RemotePort{"A", "B"})
	require.NoError(t, s.Initialize())
	var checkpoint bytes.Buffer
	require.NoError(t, ep.SaveCheckpoint(&checkpoint))
	s2, ep2 := buildWiring(t, []messaging.RemotePort{"B", "A"}, []messaging.RemotePort{"B", "A"})
	require.NoError(t, s2.Initialize())
	require.Error(t, ep2.LoadCheckpoint(&checkpoint))
	t.Log("Ordered device wiring is validated during middleware initialization " +
		"and changing the declared order invalidates the endpoint checkpoint")
}
func TestBindingRejectsUnownedDuplicateAndConnectedPorts(t *testing.T) {
	s, ep := buildWiring(t, []messaging.RemotePort{"A"}, []messaging.RemotePort{"A"})
	require.Panics(t, func() { DeviceConnection(ep).BindPort(twowaybuffered.NewPort(1, 1)) })
	p := ep.Resources.devices.ports[0]
	require.Panics(t, func() { DeviceConnection(ep).BindPort(p) })
	other := MakeBuilder().WithSimulation(s).Build("Other")
	require.Panics(t, func() { DeviceConnection(other).BindPort(p) })
	require.Empty(t, other.Resources.devices.ports)
}
