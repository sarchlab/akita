package messaging

import (
	"testing"

	"github.com/sarchlab/akita/v5/hooking"
	"github.com/stretchr/testify/require"
)

type portPresenceOwner struct{ received, freed int }

func (c *portPresenceOwner) NotifyRecv(Port)     { c.received++ }
func (c *portPresenceOwner) NotifyPortFree(Port) { c.freed++ }

type portPresenceConnection struct {
	Connection
	available, sent int
}

func (c *portPresenceConnection) NotifyAvailable(Port) { c.available++ }
func (c *portPresenceConnection) NotifySend()          { c.sent++ }

type portPresenceHook struct{ incoming, outgoing []Msg }

func (h *portPresenceHook) Func(ctx hooking.HookCtx) {
	switch ctx.Pos {
	case HookPosPortMsgRetrieveIncoming:
		h.incoming = append(h.incoming, ctx.Item.(Msg))
	case HookPosPortMsgRetrieveOutgoing:
		h.outgoing = append(h.outgoing, ctx.Item.(Msg))
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
	first := registryTestMsg{MsgMeta: MsgMeta{Src: "P", Dst: "Other"}, Value: 0}
	second := registryTestMsg{MsgMeta: MsgMeta{Src: "P", Dst: "Other"}, Value: 1}
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
	for _, expected := range []Msg{first, second} {
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
	require.Equal(t, []Msg{first, second}, hook.incoming)
	require.Equal(t, []Msg{first, second}, hook.outgoing)
	t.Log("Both retrieval directions preserved FIFO order and emitted one full-to-available notification; " +
		"empty reads emitted no notifications or retrieval hooks")
}

func TestPortWithoutOwnerPanics(t *testing.T) {
	msg := registryTestMsg{MsgMeta: MsgMeta{Src: "Other", Dst: "P"}}
	for name, use := range map[string]func(p Port){
		"Deliver":          func(p Port) { p.Deliver(msg) },
		"RetrieveOutgoing": func(p Port) { p.RetrieveOutgoing() },
		"NotifyAvailable":  func(p Port) { p.NotifyAvailable() },
	} {
		p := NewPort("P", 1, 1)
		p.SetConnection(&portPresenceConnection{})
		require.PanicsWithValue(t,
			`messaging: port "P" has no owner; a component's Build binds `+
				`its ports, and any other owner must call SetOwner`,
			func() { use(p) }, name)
	}
}

func assertEmptyPortReads(t *testing.T, p Port) {
	t.Helper()
	for _, read := range []func() (Msg, bool){
		p.PeekIncoming, p.RetrieveIncoming, p.PeekOutgoing, p.RetrieveOutgoing,
	} {
		msg, ok := read()
		require.Nil(t, msg)
		require.False(t, ok)
	}
}

func assertPortPeeks(t *testing.T, p Port, first Msg) {
	t.Helper()
	for _, read := range []func() (Msg, bool){p.PeekIncoming, p.PeekOutgoing} {
		msg, ok := read()
		require.True(t, ok)
		require.Equal(t, first, msg)
	}
	require.Equal(t, 2, p.NumIncoming())
	require.Equal(t, 2, p.NumOutgoing())
}
