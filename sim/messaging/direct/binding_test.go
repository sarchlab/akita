package direct

import (
	"github.com/sarchlab/akita/v5/sim/messaging"
	"github.com/sarchlab/akita/v5/sim/messaging/twowaybuffered"
	"github.com/sarchlab/akita/v5/sim/modeling"
	"github.com/sarchlab/akita/v5/sim/timing"
	"github.com/stretchr/testify/require"
	"testing"
)

type bindingOwner struct{}

func (*bindingOwner) NotifyRecv(messaging.Port)     {}
func (*bindingOwner) NotifyPortFree(messaging.Port) {}

func TestConnectionBindingIsOneTimeAndAtomic(t *testing.T) {
	s := modeling.NewStandaloneSimulation(timing.NewSerialEngine())
	a := NewConnection("A", s, timing.GHz)
	b := NewConnection("B", s, timing.GHz)
	p := twowaybuffered.NewPort(1, 1)
	require.Panics(t, func() { a.BindPort(p) })
	require.Empty(t, a.ports.ports)
	require.Nil(t, p.Connection())
	require.Panics(t, func() { p.BindOwner((*bindingOwner)(nil), "P") })
	require.Nil(t, p.Owner())
	require.Empty(t, p.Name())
	p.BindOwner(&bindingOwner{}, "Owner.P")
	require.Panics(t, func() { p.BindConnection((*Connection)(nil)) })
	require.Nil(t, p.Connection())
	a.BindPort(p)
	require.Panics(t, func() { a.BindPort(p) })
	require.Panics(t, func() { b.BindPort(p) })
	require.Len(t, a.ports.ports, 1)
	require.Empty(t, b.ports.ports)
	require.Same(t, a, p.Connection())
	duplicate := twowaybuffered.NewPort(1, 1)
	duplicate.BindOwner(&bindingOwner{}, "Owner.P")
	require.Panics(t, func() { a.BindPort(duplicate) })
	require.Nil(t, duplicate.Connection())
	require.Len(t, a.ports.ports, 1)
	require.NoError(t, s.Initialize())
	require.Panics(t, func() { b.BindPort(duplicate) })
	require.Nil(t, duplicate.Connection())
	require.Empty(t, b.ports.ports)
	t.Log("Rejected unowned, nil, duplicate and late bindings " +
		"without changing connection membership or port attachment")
}
