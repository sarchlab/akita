package idealmemcontroller

import (
	"path/filepath"
	"testing"

	"github.com/sarchlab/akita/v5/mem"
	"github.com/sarchlab/akita/v5/mem/memprotocol"
	"github.com/sarchlab/akita/v5/sim"
	"github.com/sarchlab/akita/v5/sim/messaging"
	"github.com/sarchlab/akita/v5/sim/messaging/twowaybuffered"
	"github.com/sarchlab/akita/v5/sim/modeling/modelingtest"
	"github.com/sarchlab/akita/v5/sim/tasks"
	"github.com/stretchr/testify/require"
)

type checkpointTrace struct {
	tasks.NopObserver
	starts []tasks.Task
	ends   []tasks.Task
}

func (r *checkpointTrace) TaskStarted(task tasks.Task) { r.starts = append(r.starts, task) }
func (r *checkpointTrace) TaskEnded(task tasks.Task)   { r.ends = append(r.ends, task) }

func buildTracingCheckpoint(t *testing.T, name string, rec *checkpointTrace) (*sim.Simulation, *Comp) {
	t.Helper()
	s := sim.MakeBuilder().WithTaskTracking().WithoutSourceRecording().
		WithOutputFileName(filepath.Join(t.TempDir(), name)).Build()
	t.Cleanup(s.Terminate)
	spec := Definition.DefaultSpec
	spec.Latency, spec.Width = 5, 1
	c := Definition.Builder().WithSimulation(s).WithSpec(spec).WithResources(Resources{
		Storage: mem.MakeStorageBuilder().WithCapacity(1024).WithSimulation(s).Build("Storage"),
	}).Build("Memory")
	c.BindPort("Top", twowaybuffered.NewPort(8, 8))
	c.BindPort("Control", twowaybuffered.NewPort(8, 8))
	(&noopConn{}).BindPort(c.Ports.Top)
	(&noopConn{}).BindPort(c.Ports.Control)
	require.NoError(t, s.Initialize())
	c.TaskTracker().Subscribe(rec, c)
	return s, c
}

func TestCheckpointRestoresSimulationWithoutTaskHistory(t *testing.T) {
	rec := &checkpointTrace{}
	src, c := buildTracingCheckpoint(t, "source", rec)
	req := messaging.Msg{ID: src.NewID(), Src: "Agent", Dst: c.Ports.Top.AsRemote(),
		Payload: memprotocol.ReadReq{Address: 0, AccessByteSize: 4}}
	c.Ports.Top.Deliver(req)
	modelingtest.Tick(c)
	require.Len(t, c.State.InflightTransactions, 1)
	recvID := c.State.InflightTransactions[0].RecvTaskID
	require.NotZero(t, recvID)
	require.NotEqual(t, req.ID, recvID)
	queued := req
	queued.ID = src.NewID()
	c.Ports.Top.Deliver(queued)
	incomingID := rec.starts[len(rec.starts)-1].ID
	c.Ports.Top.Send(messaging.Msg{ID: src.NewID(), Src: c.Ports.Top.AsRemote(), Dst: "Agent",
		RspTo: req.ID, Payload: memprotocol.WriteDoneRsp{}})
	outgoingID := rec.starts[len(rec.starts)-1].ID
	require.Equal(t, tasks.KindOutgoingBuffer, rec.starts[len(rec.starts)-1].Kind)
	checkpoint := filepath.Join(t.TempDir(), "mid-request.tar.gz")
	require.NoError(t, src.SaveCheckpoint(checkpoint))
	dst, restored := buildTracingCheckpoint(t, "restored", rec)
	require.NoError(t, dst.LoadCheckpoint(checkpoint))
	require.Equal(t, recvID, restored.State.InflightTransactions[0].RecvTaskID)
	before := len(rec.starts)
	_, present := restored.Ports.Top.RetrieveOutgoing()
	require.True(t, present)
	_, present = restored.Ports.Top.RetrieveIncoming()
	require.True(t, present)
	require.Len(t, rec.starts, before, "restoration/retrieval must not recreate buffer tasks")
	for range 8 {
		modelingtest.Tick(restored)
	}
	require.Empty(t, restored.State.InflightTransactions)
	for _, end := range rec.ends {
		require.NotContains(t, []uint64{recvID, incomingID, outgoingID}, end.ID,
			"restore must not fabricate a completion without its original task record")
	}
	rsp, ok := restored.Ports.Top.RetrieveOutgoing()
	require.True(t, ok)
	require.Equal(t, req.ID, rsp.RspTo)
	require.Equal(t, []byte{0, 0, 0, 0}, rsp.Payload.(memprotocol.DataReadyRsp).Data)
	t.Logf("restored req_in=%d, incoming_buffer=%d, outgoing_buffer=%d; "+
		"ignored unknown task handles and returned read data",
		recvID, incomingID, outgoingID)
}
