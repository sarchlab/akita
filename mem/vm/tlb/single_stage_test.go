package tlb

import (
	"testing"

	"github.com/sarchlab/akita/v5/mem/vm"
	"github.com/sarchlab/akita/v5/mem/vm/addrtransprotocol"
	akitasim "github.com/sarchlab/akita/v5/sim"
	"github.com/sarchlab/akita/v5/sim/messaging"
	"github.com/sarchlab/akita/v5/sim/modeling"
	"github.com/sarchlab/akita/v5/sim/modeling/modelingtest"
	"github.com/stretchr/testify/require"
)

func newSingleStageTLB(t *testing.T) *Comp {
	t.Helper()
	sim := akitasim.MakeBuilder().Build()
	spec := DefaultSpec()
	spec.Latency = 1
	spec.NumReqPerCycle = 1

	spec.Bottom.Destinations = []messaging.RemotePort{"Walker"}
	comp := modeling.MakeBuilder[*Comp](spec).WithSimulation(sim).Build("TLB")
	ports := defaultPorts(sim, "TLB")
	comp.BindPort("Top", ports.Top)
	comp.BindPort("Bottom", ports.Bottom)
	comp.BindPort("Control", ports.Control)
	plugNoopConn(comp)
	require.NoError(t, sim.Initialize())
	return comp
}

func deliverSingleStageRequest(comp *Comp) messaging.Msg {
	req := messaging.Msg{
		ID: comp.NewID(), Src: "Requester", Dst: comp.Ports.Top.AsRemote(),
		Payload: addrtransprotocol.Req{PID: 1, VAddr: 0x1000, DeviceID: 1},
	}
	comp.Ports.Top.Deliver(req)
	return req
}

func tickSingleStageLookup(t *testing.T, comp *Comp) {
	t.Helper()
	// Admission, pipeline output (Latency=1), and post-pipeline lookup.
	for tick := range 3 {
		require.True(t, modelingtest.Tick(comp), "tick %d must make progress", tick+1)
	}
	require.Empty(t, comp.State.Pipeline.Stages())
	require.Zero(t, comp.State.BufferItems.Size())
}

func TestSingleStageTLBHitCompletes(t *testing.T) {
	comp := newSingleStageTLB(t)
	page := vm.Page{PID: 1, VAddr: 0x1000, PAddr: 0x2000, Valid: true}
	setUpdate(&comp.State.Sets[0], 0, page)
	setVisit(&comp.State.Sets[0], 0)
	req := deliverSingleStageRequest(comp)
	tickSingleStageLookup(t, comp)
	rsp, ok := comp.Ports.Top.RetrieveOutgoing()
	require.True(t, ok)
	require.Equal(t, req.ID, rsp.RspTo)
	require.Equal(t, page, rsp.Payload.(addrtransprotocol.Rsp).Page)
	_, ok = comp.Ports.Bottom.RetrieveOutgoing()
	require.False(t, ok, "a resident hit must not fetch a translation")
}

func TestSingleStageTLBMissCompletes(t *testing.T) {
	comp := newSingleStageTLB(t)
	req := deliverSingleStageRequest(comp)
	tickSingleStageLookup(t, comp)
	fetch, ok := comp.Ports.Bottom.RetrieveOutgoing()
	require.True(t, ok)
	require.Equal(t, req.Payload, fetch.Payload)
	page := vm.Page{PID: 1, VAddr: 0x1000, PAddr: 0x2000, Valid: true}
	comp.Ports.Bottom.Deliver(messaging.Msg{
		ID: comp.NewID(), Src: "Walker", Dst: comp.Ports.Bottom.AsRemote(),
		RspTo: fetch.ID, Payload: addrtransprotocol.Rsp{Page: page},
	})
	require.True(t, modelingtest.Tick(comp)) // Receive the downstream mapping.
	require.True(t, modelingtest.Tick(comp)) // Respond to the original requester.
	rsp, ok := comp.Ports.Top.RetrieveOutgoing()
	require.True(t, ok)
	require.Equal(t, req.ID, rsp.RspTo)
	require.Equal(t, page, rsp.Payload.(addrtransprotocol.Rsp).Page)
	require.Empty(t, comp.State.MSHREntries)
	require.False(t, comp.State.HasRespondingMSHR)
}

// TestTLBPipelineAcceptsEveryCycle guards against a stage-0 dwell, which made
// each request hold the pipeline entry for two cycles and halved throughput.
func TestTLBPipelineAcceptsEveryCycle(t *testing.T) {
	sim := akitasim.MakeBuilder().Build()
	spec := DefaultSpec()
	spec.Latency = 4
	spec.NumReqPerCycle = 1

	spec.Bottom.Destinations = []messaging.RemotePort{"Walker"}
	comp := modeling.MakeBuilder[*Comp](spec).WithSimulation(sim).Build("TLB")
	ports := defaultPorts(sim, "TLB")
	comp.BindPort("Top", ports.Top)
	comp.BindPort("Bottom", ports.Bottom)
	comp.BindPort("Control", ports.Control)
	plugNoopConn(comp)
	require.NoError(t, sim.Initialize())

	for i := 1; i <= spec.Latency; i++ {
		deliverSingleStageRequest(comp)
		require.True(t, modelingtest.Tick(comp))
		require.Len(t, comp.State.Pipeline.Stages(), i,
			"a new request must enter the pipeline every cycle")
	}
}
