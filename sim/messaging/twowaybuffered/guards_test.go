package twowaybuffered

import (
	"testing"

	"github.com/sarchlab/akita/v5/sim/hooking"
	"github.com/sarchlab/akita/v5/sim/messaging"
	"github.com/stretchr/testify/require"
)

type countSendHook struct{ calls int }

func (h *countSendHook) Func(hooking.HookCtx) { h.calls++ }

func TestPortCapacitiesMustBePositive(t *testing.T) {
	for _, caps := range [][2]int{{0, 1}, {-1, 1}, {1, 0}, {1, -1}} {
		require.Panics(t, func() { newTestPort(caps[0], caps[1]) })
	}
	require.NotPanics(t, func() { newTestPort(1, 1) })
}

func TestSendGuardsLeavePortUnchanged(t *testing.T) {
	p := newTestPort(2, 2)
	p.BindOwner(&portPresenceOwner{}, "Source")
	hook := &countSendHook{}
	p.AcceptHook(hook)
	valid := messaging.Msg{ID: 1, Src: "Source", Dst: "Destination"}
	require.PanicsWithValue(t, `twowaybuffered: port "Source" has no connection`,
		func() { p.Send(valid) })
	require.Zero(t, p.OutgoingBuf().Size())
	require.Zero(t, hook.calls)
	conn := &portPresenceConnection{}
	p.BindConnection(conn)
	invalid := valid
	invalid.ID = 0
	require.PanicsWithValue(t, `twowaybuffered: port "Source" cannot send message with zero ID`,
		func() { p.Send(invalid) })
	require.Zero(t, p.OutgoingBuf().Size())
	require.Zero(t, hook.calls)
	require.Zero(t, conn.sent)
	p.Send(valid) // RspTo zero is a valid request.
	require.Equal(t, 1, p.OutgoingBuf().Size())
	require.Equal(t, 1, hook.calls)
	// Check every send, even when the outgoing buffer already contains traffic.
	p.conn = nil
	require.Panics(t, func() { p.Send(valid) })
	require.Equal(t, 1, p.OutgoingBuf().Size())
	require.Equal(t, 1, hook.calls)
}
