package direct_test

import (
	"testing"

	akitasim "github.com/sarchlab/akita/v5/sim"
	"github.com/sarchlab/akita/v5/sim/messaging"
	"github.com/sarchlab/akita/v5/sim/messaging/twowaybuffered"
	"github.com/sarchlab/akita/v5/sim/timing"
	"github.com/stretchr/testify/require"
)

type bindingOwner struct{}

func (*bindingOwner) NotifyRecv(messaging.Port)     {}
func (*bindingOwner) NotifyPortFree(messaging.Port) {}

func TestConnectionBindingIsOneTimeAndAtomic(t *testing.T) {
	s := akitasim.MakeBuilder().Build()
	a := NewConnection("A", s, timing.GHz)
	b := NewConnection("B", s, timing.GHz)
	p := twowaybuffered.MakeBuilder().WithSimulation(s).Build()
	require.Panics(t, func() { a.BindPort(p) })
	require.Empty(t, a.TestPorts())
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
	require.Len(t, a.TestPorts(), 1)
	require.Empty(t, b.TestPorts())
	require.Same(t, a, p.Connection())
	duplicate := twowaybuffered.MakeBuilder().WithSimulation(s).Build()
	duplicate.BindOwner(&bindingOwner{}, "Owner.P")
	require.Panics(t, func() { a.BindPort(duplicate) })
	require.Nil(t, duplicate.Connection())
	require.Len(t, a.TestPorts(), 1)
	require.NoError(t, s.Initialize())
	require.Panics(t, func() { b.BindPort(duplicate) })
	require.Nil(t, duplicate.Connection())
	require.Empty(t, b.TestPorts())
	t.Log("Rejected unowned, nil, duplicate and late bindings " +
		"without changing connection membership or port attachment")
}

func (o *bindingOwner) Name() string                           { return "BindingOwner" }
func (o *bindingOwner) BindPort(slot string, p messaging.Port) { p.BindOwner(o, o.Name()+"."+slot) }
