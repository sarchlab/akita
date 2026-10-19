package idealmemcontroller

import (
	"fmt"
	"path/filepath"
	"testing"

	"github.com/sarchlab/akita/v5/mem"
	"github.com/sarchlab/akita/v5/mem/memprotocol"
	"github.com/sarchlab/akita/v5/sim"
	"github.com/sarchlab/akita/v5/sim/messaging"
	"github.com/sarchlab/akita/v5/sim/messaging/twowaybuffered"
	"github.com/sarchlab/akita/v5/sim/modeling"
	"github.com/sarchlab/akita/v5/sim/modeling/modelingtest"
	"github.com/sarchlab/akita/v5/sim/tasks"
	"github.com/sarchlab/akita/v5/sim/timing"
	"github.com/stretchr/testify/require"
)

type checkpointTrace struct {
	tasks.NopObserver
	starts, ends []tasks.Task
	milestones   []tasks.Milestone
}

func (r *checkpointTrace) TaskStarted(task tasks.Task)      { r.starts = append(r.starts, task) }
func (r *checkpointTrace) TaskEnded(task tasks.Task)        { r.ends = append(r.ends, task) }
func (r *checkpointTrace) MilestoneAdded(m tasks.Milestone) { r.milestones = append(r.milestones, m) }

type checkpointClock struct{}

func (checkpointClock) Handle(timing.Event) {}

func buildTracingCheckpoint(t *testing.T, enabled bool, rec *checkpointTrace) (*sim.Simulation, *Comp) {
	t.Helper()
	b := sim.MakeBuilder().WithoutSourceRecording().WithOutputPath(filepath.Join(t.TempDir(), "trace") + ".sqlite3")
	if enabled {
		b = b.WithTaskTracking()
	}
	s := b.Build()
	t.Cleanup(func() {
		s.Terminate()
	})
	spec := DefaultSpec()
	spec.Latency, spec.Width = 5, 1
	c := modeling.MakeBuilder[*Comp](spec).WithSimulation(s).WithResources(Resources{
		Storage: mem.MakeStorageBuilder().WithCapacity(1024).WithSimulation(s).Build("Storage"),
	}).Build("Memory")
	c.BindPort("Top", twowaybuffered.NewPort(8, 8))
	c.BindPort("Control", twowaybuffered.NewPort(8, 8))
	(&noopConn{}).BindPort(c.Ports.Top)
	(&noopConn{}).BindPort(c.Ports.Control)
	s.Engine().RegisterHandler("Clock", checkpointClock{})
	require.NoError(t, s.Initialize())
	if enabled {
		s.TaskTracker().Subscribe(rec, c)
	}
	return s, c
}

func TestCheckpointTaskContinuity(t *testing.T) {
	for _, savedOn := range []bool{false, true} {
		for _, restoredOn := range []bool{false, true} {
			t.Run(fmt.Sprintf("saved_%t_restored_%t", savedOn, restoredOn), func(t *testing.T) {
				checkTaskCheckpoint(t, savedOn, restoredOn)
			})
		}
	}
}

type taskCheckpointScenario struct {
	checkpoint                                     string
	req, queued                                    messaging.Msg
	root, senderID, recvID, incomingID, outgoingID uint64
	original                                       map[uint64]tasks.Task
}

func saveTaskCheckpoint(t *testing.T, savedOn bool) taskCheckpointScenario {
	t.Helper()
	sourceEvents := &checkpointTrace{}
	src, c := buildTracingCheckpoint(t, savedOn, sourceEvents)
	src.Engine().Schedule(timing.MakeEventBase(src.NewID(), 10, "Clock"))
	require.NoError(t, src.Engine().Run())
	// Detail cannot be encoded as JSON and must never be checkpointed.
	root := c.TaskTracker().StartTask(c, tasks.StartInfo{Kind: "work", What: "root", Detail: func() {}})
	req := messaging.Msg{ID: src.NewID(), Src: "Agent", Dst: c.Ports.Top.AsRemote(),
		Payload: memprotocol.ReadReq{Address: 0, AccessByteSize: 4}}
	senderID := c.TaskTracker().StartReqOut(c, req, root)
	c.Ports.Top.Deliver(req)
	modelingtest.Tick(c)
	require.Len(t, c.State.InflightTransactions, 1)
	recvID := c.State.InflightTransactions[0].RecvTaskID
	queued := req
	queued.ID = src.NewID()
	c.Ports.Top.Deliver(queued)
	priorRsp := messaging.Msg{ID: src.NewID(), Src: c.Ports.Top.AsRemote(), Dst: "Agent",
		RspTo: req.ID, Payload: memprotocol.WriteDoneRsp{}}
	c.Ports.Top.Send(priorRsp)
	original := map[uint64]tasks.Task{}
	var incomingID, outgoingID uint64
	for _, task := range sourceEvents.starts {
		task.Detail = nil
		original[task.ID] = task
		if task.MsgID == queued.ID {
			incomingID = task.ID
		}
		if task.MsgID == priorRsp.ID {
			outgoingID = task.ID
		}
	}
	checkpoint := filepath.Join(t.TempDir(), "mid-request.tar.gz")
	require.NoError(t, src.SaveCheckpoint(checkpoint))
	return taskCheckpointScenario{checkpoint: checkpoint, req: req, queued: queued,
		root: root, senderID: senderID, recvID: recvID, incomingID: incomingID,
		outgoingID: outgoingID, original: original}
}

func checkTaskCheckpoint(t *testing.T, savedOn, restoredOn bool) {
	t.Helper()
	saved := saveTaskCheckpoint(t, savedOn)
	fresh := &checkpointTrace{}
	dst, restored := buildTracingCheckpoint(t, restoredOn, fresh)
	// Loading must replace any setup-time tracker state, including in off->on mode.
	for range 100 {
		dst.NewID()
	}
	discardedID := restored.TaskTracker().StartTask(restored, tasks.StartInfo{Kind: "work", What: "discard-on-load"})
	require.NoError(t, dst.LoadCheckpoint(saved.checkpoint))
	require.Equal(t, saved.recvID, restored.State.InflightTransactions[0].RecvTaskID)
	fresh.starts, fresh.ends, fresh.milestones = nil, nil, nil
	restored.TaskTracker().EndTask(discardedID)
	require.Empty(t, fresh.ends)

	restored.Ports.Top.AddIncomingMilestone(tasks.Reason(tasks.MilestoneKindHardwareResource, "admission"))
	require.Empty(t, fresh.milestones, "no elapsed time since restored head boundary")
	_, ok := restored.Ports.Top.RetrieveIncoming()
	require.True(t, ok)
	_, ok = restored.Ports.Top.RetrieveOutgoing()
	require.True(t, ok)
	require.Empty(t, fresh.starts, "restoration/retrieval must not fabricate starts")
	for range 8 {
		modelingtest.Tick(restored)
	}
	require.Empty(t, restored.State.InflightTransactions)
	rsp, ok := restored.Ports.Top.RetrieveOutgoing()
	require.True(t, ok)
	require.Equal(t, saved.req.ID, rsp.RspTo)
	require.Equal(t, []byte{0, 0, 0, 0}, rsp.Payload.(memprotocol.DataReadyRsp).Data)
	if restoredOn {
		require.Len(t, fresh.starts, 1)
		expectedParent := uint64(0)
		if savedOn {
			expectedParent = saved.senderID
		}
		require.Equal(t, expectedParent, fresh.starts[0].ParentID, "response must resolve the saved sender")
	}
	restored.TaskTracker().EndReqOut(saved.senderID)
	restored.TaskTracker().EndTask(saved.root)
	assertCheckpointEnds(t, saved, fresh, dst.Engine().CurrentTime(), savedOn, restoredOn)
}

func assertCheckpointEnds(t *testing.T, saved taskCheckpointScenario, fresh *checkpointTrace,
	now timing.VTimeInPicoSec, savedOn, restoredOn bool,
) {
	t.Helper()
	ends := map[uint64]tasks.Task{}
	for _, task := range fresh.ends {
		ends[task.ID] = task
	}
	for _, id := range []uint64{saved.recvID, saved.incomingID, saved.outgoingID, saved.senderID, saved.root} {
		if savedOn && restoredOn {
			want := saved.original[id]
			want.EndTime = now
			require.Equal(t, want, ends[id], "restored complete end for task %d", id)
			require.Equal(t, timing.VTimeInPicoSec(10), ends[id].StartTime)
		} else {
			require.NotContains(t, ends, id)
		}
	}
	t.Logf("tracking %t->%t: restored request returned correct data; "+
		"original req_in=%d, buffer=%d, sender=%d; observed %d ends",
		savedOn, restoredOn, saved.recvID, saved.incomingID, saved.senderID, len(ends))
}
