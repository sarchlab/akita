package tlb

import (
	"testing"

	"github.com/sarchlab/akita/v5/internal/testsim"
	"github.com/sarchlab/akita/v5/mem/vm"
	"github.com/sarchlab/akita/v5/mem/vm/addrtransprotocol"
	"github.com/sarchlab/akita/v5/sim/messaging"
	"github.com/sarchlab/akita/v5/sim/modeling"
	"github.com/stretchr/testify/require"
)

type retrySendPort struct {
	messaging.Port
	checks int
}

func (p *retrySendPort) CanSend() bool {
	p.checks++
	return p.Port.CanSend()
}

func newRetryTLB(t *testing.T) (*Comp, *retrySendPort) {
	t.Helper()
	sim := testsim.New(testsim.NewClock(), false)
	spec := DefaultSpec()
	spec.NumReqPerCycle = 4
	spec.Bottom.Destinations = []messaging.RemotePort{"Walker"}
	comp := modeling.MakeBuilder[*Comp](spec).WithSimulation(sim).Build("TLB")
	ports := defaultPorts(sim, "TLB")
	top := &retrySendPort{Port: ports.Top}
	comp.BindPort("Top", top)
	comp.BindPort("Bottom", ports.Bottom)
	comp.BindPort("Control", ports.Control)
	plugNoopConn(comp)
	require.NoError(t, sim.Initialize())
	return comp, top
}

func deliverBottomRsp(comp *Comp, rspTo, vAddr uint64) {
	comp.Ports.Bottom.Deliver(messaging.Msg{
		ID: comp.NewID(), Src: "Walker", Dst: comp.Ports.Bottom.AsRemote(), RspTo: rspTo,
		Payload: addrtransprotocol.Rsp{Page: vm.Page{PID: 1, VAddr: vAddr, Valid: true}},
	})
}

func TestRespondStopsAtBlockedTopWithoutAllocatingIDs(t *testing.T) {
	comp, top := newRetryTLB(t)
	for top.Port.CanSend() {
		top.Port.Send(messaging.Msg{ID: comp.NewID(), Src: top.AsRemote(), Dst: "Requester",
			Payload: addrtransprotocol.Rsp{}})
	}
	comp.State.HasRespondingMSHR = true
	comp.State.RespondingMSHRData = mshrEntryState{Requests: []pipelineTLBReqState{
		{Msg: messaging.Msg{ID: 10, Src: "Requester"}},
		{Msg: messaging.Msg{ID: 11, Src: "Requester"}},
	}}
	before := comp.NewID()

	require.False(t, comp.Middlewares.TLB.processResponses())

	require.Equal(t, 1, top.checks)
	require.Equal(t, before+1, comp.NewID(), "a blocked response allocates no ID")
	require.Len(t, comp.State.RespondingMSHRData.Requests, 2)
}

func TestRespondNoWaiterCleanupLetsBottomParse(t *testing.T) {
	comp, _ := newRetryTLB(t)
	comp.State.HasRespondingMSHR = true
	deliverBottomRsp(comp, 99, 0x1000)

	require.True(t, comp.Middlewares.TLB.processResponses())

	require.False(t, comp.State.HasRespondingMSHR)
	_, ok := comp.Ports.Bottom.PeekIncoming()
	require.False(t, ok)
}

func TestParseBottomDrainsStaleThenStopsAtStagedResponse(t *testing.T) {
	comp, _ := newRetryTLB(t)
	var idx int
	comp.State.MSHREntries, idx = mshrAdd(comp.State.MSHREntries, comp.Spec.NumMSHREntries, 1, 0x1000)
	comp.State.MSHREntries[idx].ReqToBottom = messaging.Msg{ID: 50}
	deliverBottomRsp(comp, 98, 0x2000)
	deliverBottomRsp(comp, 99, 0x3000)
	deliverBottomRsp(comp, 50, 0x1000)
	deliverBottomRsp(comp, 97, 0x4000)

	require.True(t, comp.Middlewares.TLB.processResponses())

	require.True(t, comp.State.HasRespondingMSHR)
	require.Empty(t, comp.State.MSHREntries)
	rsp, ok := comp.Ports.Bottom.PeekIncoming()
	require.True(t, ok)
	require.Equal(t, uint64(97), rsp.RspTo)
}
