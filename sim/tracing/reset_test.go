package tracing

import (
	"testing"

	"github.com/sarchlab/akita/v5/sim/messaging"
	"github.com/sarchlab/akita/v5/sim/modeling"
	"github.com/sarchlab/akita/v5/sim/timing"
	"github.com/stretchr/testify/require"
)

type resetTestMsg struct{}

func TestRequestTaskIdentityAndCompletion(t *testing.T) {
	comp := &ibFakeComp{sim: modeling.NewStandaloneSimulation(timing.NewSerialEngine()), name: "Comp"}
	rec := &ibRecordingTracer{}
	CollectTrace(comp, rec)
	msg := messaging.Msg{ID: comp.NewID(), Payload: resetTestMsg{}}
	out := TraceReqInitiate(comp, msg, 0)
	in := TraceReqReceive(comp, msg)
	require.NotZero(t, in)
	require.NotEqual(t, msg.ID, out)
	require.NotEqual(t, msg.ID, in)
	require.NotEqual(t, out, in)
	require.Equal(t, msg.ID, rec.starts[0].MsgID)
	require.Equal(t, msg.ID, rec.starts[1].MsgID)
	require.Zero(t, rec.starts[1].ParentID, "parent resolution belongs to observers")
	comp.time = 150
	TraceReqComplete(comp, in)
	TraceReqFinalize(comp, out)
	require.Equal(t, []TaskEnd{{ID: in, Time: 150}, {ID: out, Time: 150}}, rec.ends)
	t.Logf("message %d: req_out %d, req_in %d; completion ended the returned IDs", msg.ID, out, in)
}

func TestResetUsesOwnedTaskIDs(t *testing.T) {
	comp := &ibFakeComp{sim: modeling.NewStandaloneSimulation(timing.NewSerialEngine()), name: "Comp"}
	rec := &ibRecordingTracer{}
	CollectTrace(comp, rec)
	msg := messaging.Msg{ID: comp.NewID()}
	in := TraceReqReceive(comp, msg)
	out := TraceReqInitiate(comp, msg, 0)
	comp.time = 70
	EndTaskOnReset(comp, in)
	EndTaskOnReset(comp, out)
	EndTaskOnReset(comp, 0)
	require.Equal(t, []TaskEnd{{ID: in, Time: 70}, {ID: out, Time: 70}}, rec.ends)
}

func TestUntracedRequestsAllocateNoTaskIDs(t *testing.T) {
	comp := &ibFakeComp{sim: modeling.NewStandaloneSimulation(timing.NewSerialEngine()), name: "Comp"}
	msg := messaging.Msg{ID: comp.NewID()}
	require.Zero(t, TraceReqInitiate(comp, msg, 0))
	require.Zero(t, TraceReqReceive(comp, msg))
	require.Equal(t, msg.ID+1, comp.NewID())
	TraceReqComplete(comp, 0)
	TraceReqFinalize(comp, 0)
	EndTaskOnReset(comp, 0)
}
