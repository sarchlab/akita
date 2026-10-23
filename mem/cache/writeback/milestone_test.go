package writeback

import (
	"testing"

	"github.com/sarchlab/akita/v5/internal/testsim"
	"github.com/sarchlab/akita/v5/mem"
	"github.com/sarchlab/akita/v5/mem/memprotocol"
	"github.com/sarchlab/akita/v5/sim/messaging"
	"github.com/sarchlab/akita/v5/sim/messaging/direct"
	"github.com/sarchlab/akita/v5/sim/modeling"
	"github.com/sarchlab/akita/v5/sim/tasks"
	"github.com/sarchlab/akita/v5/sim/timing"
	"github.com/stretchr/testify/require"
)

// wbMilestoneRecorder captures the task starts, milestones, and tags the
// writeback cache emits, in emission order and without the DBTracer's
// same-key/same-time dedup, so a test can assert the buffer-vs-req_in/pipeline
// task each milestone and tag is attributed to.
type wbMilestoneRecorder struct {
	tasks.NopObserver
	starts     []tasks.Task
	milestones []tasks.Milestone
	tags       []tasks.Tag
}

func (r *wbMilestoneRecorder) TaskStarted(s tasks.Task) {
	r.starts = append(r.starts, s)
}

func (r *wbMilestoneRecorder) MilestoneAdded(m tasks.Milestone) {
	r.milestones = append(r.milestones, m)
}

func (r *wbMilestoneRecorder) TagAdded(subject tasks.Tag) {
	r.tags = append(r.tags, subject)
}

// taskID returns the ID of the first recorded task start of the given kind.
func (r *wbMilestoneRecorder) taskID(kind string) uint64 {
	for _, s := range r.starts {
		if s.Kind == kind {
			return s.ID
		}
	}

	return 0
}

// startsOfKind returns every recorded task start of the given kind.
func (r *wbMilestoneRecorder) startsOfKind(kind string) []tasks.Task {
	var out []tasks.Task
	for _, s := range r.starts {
		if s.Kind == kind {
			out = append(out, s)
		}
	}

	return out
}

func (r *wbMilestoneRecorder) milestonesOn(taskID uint64) []tasks.Milestone {
	var ms []tasks.Milestone
	for _, m := range r.milestones {
		if m.TaskID == taskID {
			ms = append(ms, m)
		}
	}

	return ms
}

func TestWriteBackCacheMilestones(t *testing.T) {
	var (
		engine      timing.Engine
		sim         *testsim.Simulation
		cacheComp   *Comp
		dramTop     messaging.Port
		dramStorage *mem.Storage
		conn        *direct.Connection
		agentPort   messaging.Port
		topPort     messaging.Port
		rec         *wbMilestoneRecorder
	)

	setup1 := func(t *testing.T) {
		t.Helper()

		engine = timing.NewSerialEngine()
		sim = testsim.New(engine, true)

		agentPort = newDriverPort(sim, "Agent.Top", 8)

		dramStorage = mem.NewStorage(4 * mem.GB)
		dramTop = buildIdealDRAM(sim, dramStorage)

		addressToPortMapper := dramTop.AsRemote()

		cacheSpec := DefaultSpec()
		cacheSpec.NumSets = 1024
		cacheSpec.NumReqPerCycle = 4

		cacheSpec.DirectoryWidth = 4
		cacheSpec.BankWidth = 4
		cacheSpec.Bottom.Destinations = []messaging.RemotePort{addressToPortMapper}
		cacheComp = modeling.MakeBuilder[*Comp](cacheSpec).
			WithSimulation(sim).
			WithResources(Resources{Storage: mem.NewStorage(cacheSpec.ByteSize())}).
			Build("Cache")
		cacheCompPorts := makePorts(sim, "Cache", 8)
		cacheComp.BindPort("Top", cacheCompPorts.Top)
		cacheComp.BindPort("Bottom", cacheCompPorts.Bottom)
		cacheComp.BindPort("Control", cacheCompPorts.Control)

		topPort = cacheComp.Ports.Top

		conn = direct.NewConnection("Connection", sim, timing.GHz)
		conn.BindPort(topPort)
		conn.BindPort(cacheComp.Ports.Bottom)
		conn.BindPort(cacheComp.Ports.Control)
		conn.BindPort(dramTop)
		conn.BindPort(agentPort)
		if err := sim.Initialize(); err != nil {
			panic(err)
		}

		// Attach the recorder before driving so request starts allocate real
		// receiver-side task IDs (it returns 0 when there are no hooks). The
		// admission milestone lands on the Top-port buffer task, so the test
		// also needs the incoming-buffer task to exist.
		rec = &wbMilestoneRecorder{}
		cacheComp.TaskTracker().Subscribe(rec, cacheComp)
	}

	t.Run("records the dir-stage-buf admission milestone on the buffer task "+
		"and a directory-pipeline subtask parented to req_in for a read miss", func(t *testing.T) {
		setup1(t)

		dramStorage.Write(0x10000, []byte{
			1, 2, 3, 4, 5, 6, 7, 8,
			1, 2, 3, 4, 5, 6, 7, 8,
			1, 2, 3, 4, 5, 6, 7, 8,
			1, 2, 3, 4, 5, 6, 7, 8,
			1, 2, 3, 4, 5, 6, 7, 8,
			1, 2, 3, 4, 5, 6, 7, 8,
			1, 2, 3, 4, 5, 6, 7, 8,
			1, 2, 3, 4, 5, 6, 7, 8,
		})

		read := messaging.Msg{Payload: memprotocol.ReadReq{
			Address:        0x10004,
			AccessByteSize: 4},
			ID:  sim.NewID(),
			Src: agentPort.AsRemote(),
			Dst: topPort.AsRemote(),

			TrafficBytes: 12,
		}

		topPort.Deliver(read)

		require.NoError(t, engine.Run())

		rsp, _ := agentPort.RetrieveIncoming()
		dr := rsp
		require.Equal(t, []byte{5, 6, 7, 8}, dr.Payload.(memprotocol.DataReadyRsp).Data)
		require.Equal(t, read.ID, dr.RspTo)

		bufID := rec.taskID(tasks.KindIncomingBuffer)
		reqInID := rec.taskID("req_in")
		require.NotZero(t, bufID)
		require.NotZero(t, reqInID)
		require.NotEqual(t, reqInID, bufID)

		// The admission milestone (left the Top buffer because the directory
		// stage buffer had room) lands on the incoming-buffer task, after the
		// queue reached-head milestone the port hook emits.
		found := false
		for _, m := range rec.milestonesOn(bufID) {
			if m.Kind == tasks.MilestoneKindHardwareResource {
				require.Equal(t, cacheComp.Name()+".dir_stage_buf", m.What)
				found = true
			}
		}
		require.True(t, found, "expected a hardware_resource admission milestone on the buffer task")

		// A directory-pipeline subtask exists and is parented to the
		// request's req_in task, closing the retrieve->directory gap.
		pipeStarts := rec.startsOfKind(tasks.KindPipeline)
		require.NotEmpty(t, pipeStarts)
		require.Equal(t, reqInID, pipeStarts[0].ParentID)
		require.Equal(t, cacheComp.Name()+".dir_pipeline", pipeStarts[0].What)
	})

	t.Run("records the dir-pipeline work milestone and the downstream-fetch "+
		"data milestone on req_in for a read miss", func(t *testing.T) {
		setup1(t)

		dramStorage.Write(0x10000, []byte{
			1, 2, 3, 4, 5, 6, 7, 8,
			1, 2, 3, 4, 5, 6, 7, 8,
			1, 2, 3, 4, 5, 6, 7, 8,
			1, 2, 3, 4, 5, 6, 7, 8,
			1, 2, 3, 4, 5, 6, 7, 8,
			1, 2, 3, 4, 5, 6, 7, 8,
			1, 2, 3, 4, 5, 6, 7, 8,
			1, 2, 3, 4, 5, 6, 7, 8,
		})

		read := messaging.Msg{Payload: memprotocol.ReadReq{
			Address:        0x10004,
			AccessByteSize: 4},
			ID:  sim.NewID(),
			Src: agentPort.AsRemote(),
			Dst: topPort.AsRemote(),

			TrafficBytes: 12,
		}

		topPort.Deliver(read)

		require.NoError(t, engine.Run())

		// Drive the read miss all the way through the downstream fetch
		// response: the agent must have received the data.
		rsp, _ := agentPort.RetrieveIncoming()
		dr := rsp
		require.Equal(t, []byte{5, 6, 7, 8}, dr.Payload.(memprotocol.DataReadyRsp).Data)
		require.Equal(t, read.ID, dr.RspTo)

		reqInID := rec.taskID("req_in")
		require.NotZero(t, reqInID)

		ms := rec.milestonesOn(reqInID)

		// (a) The directory-pipeline-exit work milestone lands on req_in,
		// marking the post-pipeline processing interval as work.
		haveWork := false
		for _, m := range ms {
			if m.Kind == tasks.MilestoneKindWork &&
				m.What == cacheComp.Name()+".dir_pipeline" {
				haveWork = true
			}
		}
		require.True(t, haveWork, "expected a work/.dir_pipeline milestone on req_in")

		// (b) The downstream fetch response charges a data milestone on
		// req_in, marking the data-wait resolved at fetch-response arrival.
		haveData := false
		for _, m := range ms {
			if m.Kind == tasks.MilestoneKindData &&
				m.What == cacheComp.Name()+".Bottom" {
				haveData = true
			}
		}
		require.True(t, haveData, "expected a data/.Bottom milestone on req_in after the fetch response")
	})
}
