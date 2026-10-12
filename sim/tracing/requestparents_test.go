package tracing

import (
	"database/sql"
	"path/filepath"
	"testing"

	"github.com/sarchlab/akita/v5/sim/datarecording"
	"github.com/sarchlab/akita/v5/sim/messaging"
	"github.com/stretchr/testify/require"
)

func TestRequestParentsInDBAndBackTrace(t *testing.T) {
	path := filepath.Join(t.TempDir(), "parents")
	recorder := datarecording.NewDataRecorder(path)
	defer recorder.Close()
	dbTracer := NewDBTracer(&testTimeTeller{}, recorder)
	dbTracer.StartTracing()
	back := NewBackTraceTracer(nil)
	for _, observer := range []Tracer{dbTracer, back} {
		observer.StartTask(TaskStart{ID: 101, MsgID: 7, Kind: ReqOutTaskKind, What: "req", Location: "Sender.req_out"})
		observer.StartTask(TaskStart{ID: 102, MsgID: 7, Kind: ReqInTaskKind, What: "req", Location: "Receiver.req_in"})
		observer.StartTask(TaskStart{ID: 103, MsgID: 8, Kind: IncomingBufferTaskKind,
			What: "rsp", Location: "Sender.Top.incoming", Detail: messaging.Msg{ID: 8, RspTo: 7}})
		observer.StartTask(TaskStart{ID: 104, MsgID: 99, Kind: ReqInTaskKind,
			What: "unobserved", Location: "Receiver.req_in"})
	}
	require.Equal(t, uint64(101), back.tracingTasks[102].ParentID)
	require.Equal(t, uint64(101), back.tracingTasks[103].ParentID)
	require.Zero(t, back.tracingTasks[104].ParentID)
	for _, observer := range []Tracer{dbTracer, back} {
		observer.EndTask(TaskEnd{ID: 101, Time: 10})
		observer.StartTask(TaskStart{ID: 105, MsgID: 7, Kind: ReqInTaskKind, What: "late", Location: "Receiver.req_in"})
	}
	require.Zero(t, back.tracingTasks[105].ParentID)
	require.Empty(t, back.parents.byMessage)
	require.Empty(t, back.parents.byTask)
	for _, id := range []uint64{102, 103, 104, 105} {
		dbTracer.EndTask(TaskEnd{ID: id, Time: 11})
	}
	dbTracer.Terminate()
	db, err := sql.Open("sqlite", path+".sqlite3")
	require.NoError(t, err)
	defer db.Close()
	for id, want := range map[uint64]uint64{102: 101, 103: 101, 104: 0, 105: 0} {
		var parent uint64
		require.NoError(t, db.QueryRow("SELECT ParentID FROM trace WHERE ID = ?", id).Scan(&parent))
		require.Equal(t, want, parent)
	}
	t.Log("DB and BackTraceTracer link tasks 102/103 to req_out 101; unmatched and completed requests have parent 0")
}

func TestParentTrackersAreIndependent(t *testing.T) {
	var a, b RequestTaskParents
	a.ResolveStart(TaskStart{ID: 11, MsgID: 1, Kind: ReqOutTaskKind})
	b.ResolveStart(TaskStart{ID: 21, MsgID: 1, Kind: ReqOutTaskKind})
	child := TaskStart{ID: 30, MsgID: 1, Kind: ReqInTaskKind}
	require.Equal(t, uint64(11), a.ResolveStart(child).ParentID)
	require.Equal(t, uint64(21), b.ResolveStart(child).ParentID)
	a.End(11)
	require.Zero(t, a.ResolveStart(child).ParentID)
	require.Equal(t, uint64(21), b.ResolveStart(child).ParentID)
}

func TestTransportAndBufferParents(t *testing.T) {
	var parents RequestTaskParents
	parents.ResolveStart(TaskStart{ID: 11, MsgID: 1, Kind: ReqOutTaskKind})
	for _, kind := range []string{"msg_e2e", IncomingBufferTaskKind, OutgoingBufferTaskKind} {
		for _, msg := range []messaging.Msg{{ID: 1}, {ID: 2, RspTo: 1}} {
			child := TaskStart{ID: 30, MsgID: msg.ID, Kind: kind, Detail: msg}
			require.Equal(t, uint64(11), parents.ResolveStart(child).ParentID)
		}
	}
}
