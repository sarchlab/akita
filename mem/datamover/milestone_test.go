package datamover

import (
	"testing"

	"github.com/sarchlab/akita/v5/internal/testsim"
	"github.com/sarchlab/akita/v5/mem"
	"github.com/sarchlab/akita/v5/mem/datamoverprotocol"
	"github.com/sarchlab/akita/v5/mem/idealmemcontroller"
	"github.com/sarchlab/akita/v5/sim/messaging"
	"github.com/sarchlab/akita/v5/sim/messaging/direct"
	"github.com/sarchlab/akita/v5/sim/modeling"
	"github.com/sarchlab/akita/v5/sim/tasks"
	"github.com/sarchlab/akita/v5/sim/timing"
	"github.com/stretchr/testify/require"
)

// dmTraceRecorder captures the full trace event stream the data mover emits —
// task starts/ends and milestones — in emission order and without the
// DBTracer's dedup, so a test can assert the admission milestone on the buffer
// task and that the req_in opened on the request is also ended.
type dmTraceRecorder struct {
	tasks.NopObserver
	starts     []tasks.Task
	ends       []tasks.Task
	milestones []tasks.Milestone
}

func (r *dmTraceRecorder) TaskStarted(subject tasks.Task) {
	r.starts = append(r.starts, subject)
}

func (r *dmTraceRecorder) TaskEnded(subject tasks.Task) {
	r.ends = append(r.ends, subject)
}

func (r *dmTraceRecorder) MilestoneAdded(m tasks.Milestone) {
	r.milestones = append(r.milestones, m)
}

// taskID returns the ID of the first recorded task start of the given kind.
func (r *dmTraceRecorder) taskID(kind string) uint64 {
	for _, s := range r.starts {
		if s.Kind == kind {
			return s.ID
		}
	}

	return 0
}

func (r *dmTraceRecorder) endedIDs() []uint64 {
	ids := make([]uint64, len(r.ends))
	for i, e := range r.ends {
		ids[i] = e.ID
	}

	return ids
}

func (r *dmTraceRecorder) milestonesOn(taskID uint64) []tasks.Milestone {
	var ms []tasks.Milestone
	for _, m := range r.milestones {
		if m.TaskID == taskID {
			ms = append(ms, m)
		}
	}

	return ms
}

func TestDataMoverMilestones(t *testing.T) {
	var (
		engine         timing.Engine
		sim            *testsim.Simulation
		dataMover      *Comp
		insideMem      *idealmemcontroller.Comp
		insideStorage  *mem.Storage
		outsideMem     *idealmemcontroller.Comp
		outsideStorage *mem.Storage
		conn           *direct.Connection
		srcPort        messaging.Port
		topPort        messaging.Port
		rec            *dmTraceRecorder
	)

	setup1 := func(t *testing.T) {
		t.Helper()

		engine = timing.NewSerialEngine()
		sim = testsim.New(engine, true)

		srcPort = newDriverPort(sim, "Src.Top", 4)

		memSpec := idealmemcontroller.DefaultSpec()
		memSpec.Latency = 100
		memSpec.NumReqPerCycle = 1

		insideStorage = mem.NewStorage(1 * mem.MB)
		insideMem = buildIdealMem(sim, memSpec, insideStorage, "InsideMem")

		outsideStorage = mem.NewStorage(1 * mem.MB)
		outsideMem = buildIdealMem(sim, memSpec, outsideStorage, "OutsideMem")

		dmSpec := DefaultSpec()
		dmSpec.BufferSize = 2048
		dmSpec.InsideByteGranularity = 64
		dmSpec.OutsideByteGranularity = 256

		dmSpec.Inside.Destinations = []messaging.RemotePort{insideMem.Ports.Top.AsRemote()}

		dmSpec.Outside.Destinations = []messaging.RemotePort{outsideMem.Ports.Top.AsRemote()}
		dataMover = modeling.MakeBuilder[*Comp](dmSpec).
			WithSimulation(sim).
			Build("DataMover")
		dataMoverPorts := makePorts(sim, "DataMover", 16, 64, 64, 40960000)
		dataMover.BindPort("Top", dataMoverPorts.Top)
		dataMover.BindPort("Inside", dataMoverPorts.Inside)
		dataMover.BindPort("Outside", dataMoverPorts.Outside)
		dataMover.BindPort("Control", dataMoverPorts.Control)

		topPort = dataMover.Ports.Top

		conn = direct.NewConnection("Conn", sim, timing.GHz)
		conn.BindPort(srcPort)
		conn.BindPort(topPort)
		conn.BindPort(dataMover.Ports.Inside)
		conn.BindPort(dataMover.Ports.Outside)
		conn.BindPort(insideMem.Ports.Top)
		conn.BindPort(outsideMem.Ports.Top)
		if err := sim.Initialize(); err != nil {
			panic(err)
		}

		// Attach the recorder before driving so request and buffer starts
		// allocate real task IDs (they return 0 when there
		// are no hooks). The admission milestone lands on the Top-port buffer
		// task, so the test also needs that buffer task to exist.
		rec = &dmTraceRecorder{}
		dataMover.TaskTracker().Subscribe(rec, dataMover)
	}

	t.Run("attributes the admission wait to the buffer task and closes the "+
		"req_in that was opened on the request", func(t *testing.T) {
		setup1(t)

		data := make([]byte, 4096)
		for i := range 4096 {
			data[i] = byte(i)
		}
		outsideStorage.Write(0, data)

		req := messaging.Msg{Payload: datamoverprotocol.Req{
			SrcAddress: 0,
			SrcSide:    "outside",
			DstAddress: 0,
			DstSide:    "inside",
			ByteSize:   4096},
			ID:  sim.NewID(),
			Src: srcPort.AsRemote(),
			Dst: topPort.AsRemote(),
		}

		topPort.Deliver(req)

		require.NoError(t, engine.Run())

		// The move completed end-to-end.
		require.Equal(t, data, insideStorage.Read(0, 4096))
		value0, present0 := srcPort.RetrieveIncoming()
		require.True(t, present0)
		require.IsType(t, messaging.Msg{Payload: datamoverprotocol.Rsp{}}, value0)

		// (a) The admission milestone lands on the buffer task, not on req_in.
		bufID := rec.taskID(tasks.KindIncomingBuffer)
		reqInID := rec.taskID("req_in")
		require.NotZero(t, bufID)
		require.NotZero(t, reqInID)
		require.NotEqual(t, reqInID, bufID)

		bufMs := rec.milestonesOn(bufID)
		var admission *tasks.Milestone
		for i := range bufMs {
			if bufMs[i].Kind == tasks.MilestoneKindHardwareResource {
				admission = &bufMs[i]
			}
		}
		require.NotNil(t, admission)
		require.Equal(t, dataMover.Name()+".transaction", admission.What)

		// (b) The req_in opened on the request is also ENDED. Before the
		// close-key fix it leaked because it was "closed" on the fresh response
		// (a different key), so no EndTask carried the req_in task id.
		require.Contains(t, rec.endedIDs(), reqInID)
	})

	t.Run("charges the in-flight src-read and dst-write waits to the req_in "+
		"as data and subtask milestones", func(t *testing.T) {
		setup1(t)

		data := make([]byte, 4096)
		for i := range 4096 {
			data[i] = byte(i)
		}
		outsideStorage.Write(0, data)

		req := messaging.Msg{Payload: datamoverprotocol.Req{
			SrcAddress: 0,
			SrcSide:    "outside",
			DstAddress: 0,
			DstSide:    "inside",
			ByteSize:   4096},
			ID:  sim.NewID(),
			Src: srcPort.AsRemote(),
			Dst: topPort.AsRemote(),
		}

		topPort.Deliver(req)

		require.NoError(t, engine.Run())

		// The move completed end-to-end, so reads and writes were issued and
		// acknowledged.
		require.Equal(t, data, insideStorage.Read(0, 4096))
		value1, present1 := srcPort.RetrieveIncoming()
		require.True(t, present1)
		require.IsType(t, messaging.Msg{Payload: datamoverprotocol.Rsp{}}, value1)

		reqInID := rec.taskID("req_in")
		require.NotZero(t, reqInID)

		reqInMs := rec.milestonesOn(reqInID)

		// The src-read waits are charged as data milestones on the src port.
		var hasData, hasSubTask bool
		for _, ms := range reqInMs {
			if ms.Kind == tasks.MilestoneKindData &&
				ms.What == dataMover.Ports.Outside.Name() {
				hasData = true
			}
			if ms.Kind == tasks.MilestoneKindSubTask &&
				ms.What == dataMover.Ports.Inside.Name() {
				hasSubTask = true
			}
		}
		require.True(t, hasData, "req_in should carry a data milestone for the src read wait")
		require.True(t, hasSubTask, "req_in should carry a subtask milestone for the dst write wait")
	})

	t.Run("waits at the head and admits a request that arrives while a "+
		"transaction is running, instead of dropping it", func(t *testing.T) {
		setup1(t)

		data := make([]byte, 4096)
		for i := range 4096 {
			data[i] = byte(i)
		}
		outsideStorage.Write(0, data)

		makeMove := func() messaging.Msg {
			req := messaging.Msg{Payload: datamoverprotocol.Req{
				SrcAddress: 0,
				SrcSide:    "outside",
				DstAddress: 0,
				DstSide:    "inside",
				ByteSize:   4096},
				ID:  sim.NewID(),
				Src: srcPort.AsRemote(),
				Dst: topPort.AsRemote(),
			}

			return req
		}

		// Both requests are delivered into the Top buffer before the data mover
		// runs, so the second sits behind the first (which becomes active first).
		// Before the reorder fix the data mover would retrieve-and-drop the
		// second while the first was active, silently losing it.
		req1 := makeMove()
		req2 := makeMove()
		topPort.Deliver(req1)
		topPort.Deliver(req2)

		require.NoError(t, engine.Run())

		// Both transactions completed: two responses come back to the source.
		value2, present2 := srcPort.RetrieveIncoming()
		require.True(t, present2)
		require.IsType(t, messaging.Msg{Payload: datamoverprotocol.Rsp{}}, value2)
		value3, present3 := srcPort.RetrieveIncoming()
		require.True(t, present3)
		require.IsType(t, messaging.Msg{Payload: datamoverprotocol.Rsp{}}, value3)

		// Two req_in tasks opened and both were closed (one per request), so the
		// second request was processed, not dropped.
		var reqInStarts, reqInEnds int
		reqInIDs := map[uint64]bool{}
		for _, s := range rec.starts {
			if s.Kind == "req_in" {
				reqInStarts++
				reqInIDs[s.ID] = true
			}
		}
		for _, e := range rec.endedIDs() {
			if reqInIDs[e] {
				reqInEnds++
			}
		}
		require.Equal(t, 2, reqInStarts)
		require.Equal(t, 2, reqInEnds)
	})
}
