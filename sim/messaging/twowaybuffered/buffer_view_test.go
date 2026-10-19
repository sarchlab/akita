package twowaybuffered

import (
	"bytes"
	"sync"
	"testing"

	"github.com/sarchlab/akita/v5/sim/messaging"
	"github.com/stretchr/testify/require"
)

func TestBufferViewsObserveTrafficAndRestoration(t *testing.T) {
	p := newTestPort(2, 3)
	owner := &portPresenceOwner{}
	owner.BindPort("P", p)
	conn := &portPresenceConnection{}
	p.BindConnection(conn)
	in, out := p.IncomingBuf(), p.OutgoingBuf()
	require.NotNil(t, in)
	require.NotNil(t, out)
	require.Zero(t, in.Size())
	require.Zero(t, out.Size())
	require.Equal(t, 2, in.Capacity())
	require.Equal(t, 3, out.Capacity())
	msg := messaging.Msg{ID: 1, Src: p.AsRemote(), Dst: "Other"}
	p.Send(msg)
	require.Same(t, p, conn.lastSender)
	p.Deliver(msg)
	var saved bytes.Buffer
	require.NoError(t, p.SaveCheckpoint(&saved))
	p.RetrieveIncoming()
	p.RetrieveOutgoing()
	require.Zero(t, in.Size())
	require.Zero(t, out.Size())
	require.NoError(t, p.LoadCheckpoint(&saved))
	require.Equal(t, 1, in.Size())
	require.Equal(t, 1, out.Size())
	require.Equal(t, 2, in.Capacity())
	require.Equal(t, 3, out.Capacity())
	t.Log("Retained buffer views reported 0 -> 1 -> 0 -> 1 across traffic and checkpoint restore; " +
		"capacities stayed 2 and 3")
}

func TestBufferViewsSynchronizeWithTraffic(t *testing.T) {
	p := newTestPort(2, 3)
	(&portPresenceOwner{}).BindPort("P", p)
	p.BindConnection(&portPresenceConnection{})
	in, out := p.IncomingBuf(), p.OutgoingBuf()
	var wg sync.WaitGroup
	wg.Add(1)
	go func() {
		defer wg.Done()
		for range 1000 {
			_ = in.Size()
			_ = out.Size()
			_ = in.Capacity()
			_ = out.Capacity()
		}
	}()
	for range 1000 {
		p.Send(messaging.Msg{ID: 1, Src: p.AsRemote(), Dst: "Other"})
		p.RetrieveOutgoing()
		p.Deliver(messaging.Msg{})
		p.RetrieveIncoming()
	}
	wg.Wait()
}
