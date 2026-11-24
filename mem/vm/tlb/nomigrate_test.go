package tlb

import (
	"testing"

	"github.com/sarchlab/akita/v5/mem/vm"
	"github.com/sarchlab/akita/v5/mem/vm/addrtransprotocol"
	"github.com/sarchlab/akita/v5/sim/messaging"
	"github.com/stretchr/testify/require"
)

func transReqItem(comp *Comp, noMigrate bool) pipelineTLBReqState {
	return pipelineTLBReqState{Msg: messaging.Msg{
		ID: comp.NewID(), Src: "Requester", Dst: comp.Ports.Top.AsRemote(),
		Payload: addrtransprotocol.Req{
			PID: 1, VAddr: 0x1000, DeviceID: 1, NoMigrate: noMigrate,
		},
	}}
}

// TestNoMigrateMissIsForwardedAndNotMerged checks that a miss forwards
// NoMigrate to the bottom and that a NoMigrate miss and a normal miss of one
// page do not share an MSHR entry or a result.
func TestNoMigrateMissIsForwardedAndNotMerged(t *testing.T) {
	comp, _ := newRetryTLB(t)
	mw := comp.Middlewares.TLB

	normal := transReqItem(comp, false)
	require.True(t, mw.lookup(normal))
	normalFetch, ok := comp.Ports.Bottom.RetrieveOutgoing()
	require.True(t, ok)
	require.False(t, normalFetch.Payload.(addrtransprotocol.Req).NoMigrate)

	noMigrate := transReqItem(comp, true)
	require.True(t, mw.lookup(noMigrate))
	noMigrateFetch, ok := comp.Ports.Bottom.RetrieveOutgoing()
	require.True(t, ok, "a NoMigrate miss must not merge into a normal miss")
	fetched := noMigrateFetch.Payload.(addrtransprotocol.Req)
	require.True(t, fetched.NoMigrate)
	require.Equal(t, vm.PID(1), fetched.PID)
	require.Equal(t, uint64(0x1000), fetched.VAddr)

	// A second NoMigrate miss merges into the NoMigrate entry.
	noMigrate2 := transReqItem(comp, true)
	require.True(t, mw.lookup(noMigrate2))
	_, ok = comp.Ports.Bottom.RetrieveOutgoing()
	require.False(t, ok)
	require.Len(t, comp.State.MSHREntries, 2)

	// The NoMigrate result answers only the NoMigrate requests.
	deliverBottomRsp(comp, noMigrateFetch.ID, 0x1000)
	require.True(t, mw.parseBottom())
	require.True(t, comp.State.HasRespondingMSHR)
	responding := comp.State.RespondingMSHRData.Requests
	require.Len(t, responding, 2)
	require.Equal(t, noMigrate.Msg.ID, responding[0].Msg.ID)
	require.Equal(t, noMigrate2.Msg.ID, responding[1].Msg.ID)

	require.Len(t, comp.State.MSHREntries, 1)
	remaining := comp.State.MSHREntries[0]
	require.False(t, remaining.NoMigrate)
	require.Equal(t, normalFetch.ID, remaining.ReqToBottom.ID)
	require.Len(t, remaining.Requests, 1)
	require.Equal(t, normal.Msg.ID, remaining.Requests[0].Msg.ID)
}
