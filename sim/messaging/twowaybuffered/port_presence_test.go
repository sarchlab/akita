package twowaybuffered

import (
	"testing"

	"github.com/sarchlab/akita/v5/sim/messaging"

	"github.com/sarchlab/akita/v5/sim/hooking"
	"github.com/stretchr/testify/require"
)

type portPresenceOwner struct{ received, freed int }

func (c *portPresenceOwner) NotifyRecv(messaging.Port)     { c.received++ }
func (c *portPresenceOwner) NotifyPortFree(messaging.Port) { c.freed++ }

type portPresenceConnection struct {
	messaging.Connection
	available, sent int
}

func (c *portPresenceConnection) NotifyAvailable(messaging.Port) { c.available++ }
func (c *portPresenceConnection) NotifySend()                    { c.sent++ }

type portPresenceHook struct{ incoming, outgoing []messaging.Msg }

func (h *portPresenceHook) Func(ctx hooking.HookCtx) {
	switch ctx.Pos {
	case messaging.HookPosPortMsgRetrieveIncoming:
		h.incoming = append(h.incoming, ctx.Item.(messaging.Msg))
	case messaging.HookPosPortMsgRetrieveOutgoing:
		h.outgoing = append(h.outgoing, ctx.Item.(messaging.Msg))
	}
}

func TestPortReadPresenceAndNotifications(t *testing.T) {
	comp := &portPresenceOwner{}
	conn := &portPresenceConnection{}
	p := NewPort("P", 2, 2)
	p.SetOwner(comp)
	p.SetConnection(conn)
	hook := &portPresenceHook{}
	p.AcceptHook(hook)
	first := messaging.Msg{Src: "P", Dst: "Other", Payload: registryTestMsg{Value: 0}}
	second := messaging.Msg{Src: "P", Dst: "Other", Payload: registryTestMsg{Value: 1}}
	assertEmptyPortReads(t, p)
	require.Zero(t, conn.available)
	require.Zero(t, comp.freed)
	require.Empty(t, hook.incoming)
	require.Empty(t, hook.outgoing)

	p.Deliver(first)
	p.Deliver(second)
	p.Send(first)
	p.Send(second)
	require.Equal(t, 1, comp.received)
	require.Equal(t, 1, conn.sent)
	assertPortPeeks(t, p, first)
	require.Zero(t, conn.available)
	require.Zero(t, comp.freed)
	require.Empty(t, hook.incoming)
	require.Empty(t, hook.outgoing)
	for _, expected := range []messaging.Msg{first, second} {
		msg, ok := p.RetrieveIncoming()
		require.True(t, ok)
		require.Equal(t, expected, msg)
		msg, ok = p.RetrieveOutgoing()
		require.True(t, ok)
		require.Equal(t, expected, msg)
		require.Equal(t, 1, conn.available)
		require.Equal(t, 1, comp.freed)
	}
	assertEmptyPortReads(t, p)
	require.Equal(t, 1, conn.available)
	require.Equal(t, 1, comp.freed)
	require.Equal(t, []messaging.Msg{first, second}, hook.incoming)
	require.Equal(t, []messaging.Msg{first, second}, hook.outgoing)
	t.Log("Both retrieval directions preserved FIFO order and emitted one full-to-available notification; " +
		"empty reads emitted no notifications or retrieval hooks")
}

func TestPortWithoutOwnerPanics(t *testing.T) {
	msg := messaging.Msg{Src: "Other", Dst: "P", Payload: registryTestMsg{}}
	for name, use := range map[string]func(p messaging.Port){
		"Deliver":          func(p messaging.Port) { p.Deliver(msg) },
		"RetrieveOutgoing": func(p messaging.Port) { p.RetrieveOutgoing() },
		"NotifyAvailable":  func(p messaging.Port) { p.NotifyAvailable() },
	} {
		p := NewPort("P", 1, 1)
		p.SetConnection(&portPresenceConnection{})
		require.PanicsWithValue(t,
			`twowaybuffered: port "P" has no owner; a component's Build binds `+
				`its ports, and any other owner must call SetOwner`,
			func() { use(p) }, name)
	}
}

func assertEmptyPortReads(t *testing.T, p messaging.Port) {
	t.Helper()
	for _, read := range []func() (messaging.Msg, bool){
		p.PeekIncoming, p.RetrieveIncoming, p.PeekOutgoing, p.RetrieveOutgoing,
	} {
		msg, ok := read()
		require.Equal(t, messaging.Msg{}, msg)
		require.False(t, ok)
	}
}

func assertPortPeeks(t *testing.T, p messaging.Port, first messaging.Msg) {
	t.Helper()
	for _, read := range []func() (messaging.Msg, bool){p.PeekIncoming, p.PeekOutgoing} {
		msg, ok := read()
		require.True(t, ok)
		require.Equal(t, first, msg)
	}
	require.Equal(t, 2, p.NumIncoming())
	require.Equal(t, 2, p.NumOutgoing())
}
