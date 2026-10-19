package writethrough_test

import (
	"testing"

	"github.com/sarchlab/akita/v5/internal/testsim"
	"github.com/sarchlab/akita/v5/mem"
	. "github.com/sarchlab/akita/v5/mem/cache/writethrough"
	"github.com/sarchlab/akita/v5/mem/idealmemcontroller"
	"github.com/sarchlab/akita/v5/mem/memprotocol"
	"github.com/sarchlab/akita/v5/sim/messaging"
	"github.com/sarchlab/akita/v5/sim/messaging/direct"
	"github.com/sarchlab/akita/v5/sim/messaging/twowaybuffered"
	"github.com/sarchlab/akita/v5/sim/modeling"
	"github.com/sarchlab/akita/v5/sim/tasks"
	"github.com/sarchlab/akita/v5/sim/timing"
	"github.com/stretchr/testify/require"
)

// milestoneRecorder captures the task starts, milestones, and tags the cache
// emits, in emission order and without any same-key/same-time dedup, so a test
// can assert the full ordered set of blocking reasons and the task (buffer vs
// req_in vs pipeline subtask) each one is attributed to. Mirrors the recorder
// used in mem/rob/milestone_test.go.
type milestoneRecorder struct {
	tasks.NopObserver
	starts     []tasks.Task
	milestones []tasks.Milestone
	tags       []tasks.Tag
}

func (r *milestoneRecorder) TaskStarted(s tasks.Task) {
	r.starts = append(r.starts, s)
}

func (r *milestoneRecorder) MilestoneAdded(m tasks.Milestone) {
	r.milestones = append(r.milestones, m)
}

func (r *milestoneRecorder) TagAdded(subject tasks.Tag) {
	r.tags = append(r.tags, subject)
}

// firstStart returns the first recorded task start of the given kind.
func (r *milestoneRecorder) firstStart(kind string) (tasks.Task, bool) {
	for _, s := range r.starts {
		if s.Kind == kind {
			return s, true
		}
	}

	return tasks.Task{}, false
}

func (r *milestoneRecorder) milestonesOn(taskID uint64) []tasks.Milestone {
	var ms []tasks.Milestone
	for _, m := range r.milestones {
		if m.TaskID == taskID {
			ms = append(ms, m)
		}
	}

	return ms
}

func (r *milestoneRecorder) hasMilestone(taskID uint64, what string) bool {
	for _, m := range r.milestonesOn(taskID) {
		if m.What == what {
			return true
		}
	}

	return false
}

func (r *milestoneRecorder) tagsWith(what string) []tasks.Tag {
	var ts []tasks.Tag
	for _, subject := range r.tags {
		if subject.What == what {
			ts = append(ts, subject)
		}
	}

	return ts
}

func TestCacheMilestones(t *testing.T) {
	var (
		engine      timing.Engine
		sim         *testsim.Simulation
		connection  messaging.Connection
		dram        *idealmemcontroller.Comp
		dramStorage *mem.Storage
		cuPort      messaging.Port
		c           *Comp
		rec         *milestoneRecorder
	)

	buildCache := func(policy string) {
		engine = timing.NewSerialEngine()
		sim = testsim.New(engine, true)
		connection = direct.NewConnection("Conn", sim, timing.GHz)

		cuPort = newDriverPort(sim, "CU.Top", 16)

		dramStorage = mem.NewStorage(4 * mem.GB)
		dram = modeling.MakeBuilder[*idealmemcontroller.Comp](idealmemcontroller.DefaultSpec()).
			WithSimulation(sim).
			WithResources(idealmemcontroller.Resources{Storage: dramStorage}).
			Build("DRAM")

		dram.BindPort("Top", twowaybuffered.MakeBuilder().
			WithSimulation(sim).
			WithSpec(twowaybuffered.Spec{IncomingCapacity: 16, OutgoingCapacity: 16}).
			Build())
		dram.BindPort("Control", twowaybuffered.MakeBuilder().
			WithSimulation(sim).
			WithSpec(twowaybuffered.Spec{IncomingCapacity: 16, OutgoingCapacity: 16}).
			Build())

		addressToPortMapper := dram.Ports.Top.AsRemote()

		spec := DefaultSpec()
		spec.WritePolicyType = policy

		spec.Bottom.Destinations = []messaging.RemotePort{addressToPortMapper}
		c = modeling.MakeBuilder[*Comp](spec).
			WithSimulation(sim).
			WithResources(Resources{Storage: mem.NewStorage(spec.TotalByteSize)}).
			Build("Cache")

		c.BindPort("Top", twowaybuffered.MakeBuilder().
			WithSimulation(sim).
			WithSpec(twowaybuffered.Spec{IncomingCapacity: 4, OutgoingCapacity: 4}).
			Build())
		c.BindPort("Bottom", twowaybuffered.MakeBuilder().
			WithSimulation(sim).
			WithSpec(twowaybuffered.Spec{IncomingCapacity: 4, OutgoingCapacity: 4}).
			Build())
		c.BindPort("Control", twowaybuffered.MakeBuilder().
			WithSimulation(sim).
			WithSpec(twowaybuffered.Spec{IncomingCapacity: 4, OutgoingCapacity: 4}).
			Build())

		connection.BindPort(dram.Ports.Top)
		connection.BindPort(c.Ports.Top)
		connection.BindPort(c.Ports.Bottom)
		connection.BindPort(cuPort)
		if err := sim.Initialize(); err != nil {
			panic(err)
		}

		rec = &milestoneRecorder{}
		c.TaskTracker().Subscribe(rec, c)
		// The admission milestones and the buffer task land on the Top-port
		// buffer task, so the test needs that task to exist.
	}

	drainResponses := func() []messaging.Msg {
		msgs := []messaging.Msg{}
		for {
			msg, ok := cuPort.RetrieveIncoming()
			if !ok {
				break
			}
			msgs = append(msgs, msg)
		}
		return msgs
	}

	t.Run("records admission milestones on the buffer task and a directory "+
		"pipeline subtask parented to req_in for a read miss", func(t *testing.T) {
		buildCache("write-around")

		dramStorage.Write(0x100, []byte{1, 2, 3, 4})
		read := messaging.Msg{Payload: memprotocol.ReadReq{Address: 0x100, AccessByteSize: 4},
			ID:           sim.NewID(),
			Src:          cuPort.AsRemote(),
			Dst:          c.Ports.Top.AsRemote(),
			TrafficBytes: 12,
		}

		c.Ports.Top.Deliver(read)

		require.NoError(t, engine.Run())

		rsps := drainResponses()
		require.Len(t, rsps, 1)
		require.Equal(t, []byte{1, 2, 3, 4}, rsps[0].Payload.(memprotocol.DataReadyRsp).Data)

		// The buffer task records the transaction-slot wait. The directory
		// slot is ready at that same timestamp, so it adds no interval.
		bufStart, ok := rec.firstStart(tasks.KindIncomingBuffer)
		require.True(t, ok)
		require.True(t, rec.hasMilestone(bufStart.ID, "Cache.trans"))
		require.False(t, rec.hasMilestone(bufStart.ID, "Cache.dir_buf"))

		// req_in is the cache's receiver-side task for the read request. Its
		// ID comes from the recorded task start; the transaction is already done.
		reqInStart, ok := rec.firstStart("req_in")
		require.True(t, ok)
		require.NotZero(t, reqInStart.ID)

		// The directory pipeline subtask exists and is parented to req_in,
		// per the buffer-task convention.
		pipeStart, ok := rec.firstStart(tasks.KindPipeline)
		require.True(t, ok)
		require.Equal(t, "Cache.dir_pipeline", pipeStart.What)
		require.Equal(t, reqInStart.ID, pipeStart.ParentID)

		// The cache no longer opens a separate cache_transaction task: the
		// req_in is the single task that carries the request's tags, its
		// processing milestones, and its downstream req_out children.
		_, ok = rec.firstStart("cache_transaction")
		require.False(t, ok)
	})

	t.Run("records the dir_pipeline work and bottom data milestones on req_in "+
		"for a read miss driven through the downstream fetch response", func(t *testing.T) {
		buildCache("write-around")

		dramStorage.Write(0x100, []byte{1, 2, 3, 4})
		read := messaging.Msg{Payload: memprotocol.ReadReq{Address: 0x100, AccessByteSize: 4},
			ID:           sim.NewID(),
			Src:          cuPort.AsRemote(),
			Dst:          c.Ports.Top.AsRemote(),
			TrafficBytes: 12,
		}

		c.Ports.Top.Deliver(read)

		// Run to quiescence: the read miss fetches from the lower memory and the
		// DataReadyRsp comes back, filling the line and completing the request.
		require.NoError(t, engine.Run())

		rsps := drainResponses()
		require.Len(t, rsps, 1)
		require.Equal(t, []byte{1, 2, 3, 4}, rsps[0].Payload.(memprotocol.DataReadyRsp).Data)

		reqInStart, ok := rec.firstStart("req_in")
		require.True(t, ok)
		require.NotZero(t, reqInStart.ID)

		// WORK milestone: emitted on the req_in at the directory pipeline exit,
		// before the lookup's same-tick processing milestones.
		require.True(t, rec.hasMilestone(reqInStart.ID, "Cache.dir_pipeline"))
		for _, m := range rec.milestonesOn(reqInStart.ID) {
			if m.What == "Cache.dir_pipeline" {
				require.Equal(t, tasks.MilestoneKindWork, m.Kind)
			}
		}

		// DATA milestone: emitted on the req_in when the downstream read fill
		// (DataReadyRsp) arrives, keyed to the req_in, not the response.
		require.True(t, rec.hasMilestone(reqInStart.ID, "Cache.Bottom"))
		for _, m := range rec.milestonesOn(reqInStart.ID) {
			if m.What == "Cache.Bottom" {
				require.Equal(t, tasks.MilestoneKindData, m.Kind)
			}
		}
	})

	t.Run("tags a write-through write hit with write-hit", func(t *testing.T) {
		buildCache("write-through")

		// Prime the line into the cache with a full-line write so the
		// follow-up partial write lands on a valid block (a write hit).
		fullLine := make([]byte, 64)
		for i := range fullLine {
			fullLine[i] = byte(i)
		}
		warm := messaging.Msg{Payload: memprotocol.WriteReq{Address: 0x100, Data: fullLine},
			ID:           sim.NewID(),
			Src:          cuPort.AsRemote(),
			Dst:          c.Ports.Top.AsRemote(),
			TrafficBytes: 64 + 12,
		}

		c.Ports.Top.Deliver(warm)
		require.NoError(t, engine.Run())
		require.Len(t, drainResponses(), 1)

		// Discard the warm write's trace events so the assertions below see only
		// the partial-write-hit transaction.
		rec.starts = nil
		rec.milestones = nil
		rec.tags = nil

		// Now a partial write to the same line: this is a write-through write
		// hit and must carry the write-hit tag.
		hit := messaging.Msg{Payload: memprotocol.WriteReq{Address: 0x100, Data: []byte{9, 9, 9, 9}},
			ID:           sim.NewID(),
			Src:          cuPort.AsRemote(),
			Dst:          c.Ports.Top.AsRemote(),
			TrafficBytes: 4 + 12,
		}

		c.Ports.Top.Deliver(hit)
		require.NoError(t, engine.Run())
		require.Len(t, drainResponses(), 1)

		// The req_in is now the cache's single task for the request, so the
		// write-hit tag the directory emits on writethroughWriteHit hangs off
		// the req_in — there is no separate cache_transaction.
		reqInStart, ok := rec.firstStart("req_in")
		require.True(t, ok)
		require.Equal(t, "WriteReq", reqInStart.What)

		_, ctOK := rec.firstStart("cache_transaction")
		require.False(t, ctOK)

		writeHitTags := rec.tagsWith("write-hit")
		require.NotEmpty(t, writeHitTags)

		found := false
		for _, subject := range writeHitTags {
			if subject.TaskID == reqInStart.ID {
				found = true
			}
		}
		require.True(t, found)

		// The write-through write dispatched a downstream WriteReq whose
		// WriteDoneRsp came back during this run. processDoneRsp records a
		// dependency milestone on the write's req_in (not the ack).
		require.True(t, rec.hasMilestone(reqInStart.ID, "Cache.Bottom"))
		for _, m := range rec.milestonesOn(reqInStart.ID) {
			if m.What == "Cache.Bottom" {
				require.Equal(t, tasks.MilestoneKindDependency, m.Kind)
			}
		}
	})

	t.Run("tags a write-through full-line write miss with write-miss only", func(t *testing.T) {
		buildCache("write-through")

		// A full-line write to a cold line is a write miss handled by the
		// full-line fast path, which installs a victim way and writes it. That
		// install must not also stamp a write-hit tag: the transaction is a
		// miss and the visualization shows one categorical label, not two
		// contradictory ones.
		fullLine := make([]byte, 64)
		for i := range fullLine {
			fullLine[i] = byte(i)
		}
		miss := messaging.Msg{Payload: memprotocol.WriteReq{Address: 0x100, Data: fullLine},
			ID:           sim.NewID(),
			Src:          cuPort.AsRemote(),
			Dst:          c.Ports.Top.AsRemote(),
			TrafficBytes: 64 + 12,
		}

		c.Ports.Top.Deliver(miss)
		require.NoError(t, engine.Run())
		require.Len(t, drainResponses(), 1)

		reqInStart, ok := rec.firstStart("req_in")
		require.True(t, ok)
		require.Equal(t, "WriteReq", reqInStart.What)

		// The req_in (the cache's single task for the request) carries
		// write-miss...
		missTagged := false
		for _, subject := range rec.tagsWith("write-miss") {
			if subject.TaskID == reqInStart.ID {
				missTagged = true
			}
		}
		require.True(t, missTagged)

		// ...and must NOT also carry write-hit.
		for _, subject := range rec.tagsWith("write-hit") {
			require.NotEqual(t, reqInStart.ID, subject.TaskID)
		}
	})

	t.Run("records the bank access as a subtask parented to req_in, paired "+
		"with a work milestone", func(t *testing.T) {
		buildCache("write-through")

		// A full-line write to a cold line installs a victim way and writes
		// the bank, so the transaction traverses the bank pipeline and must
		// open a bank subtask.
		fullLine := make([]byte, 64)
		w := messaging.Msg{Payload: memprotocol.WriteReq{Address: 0x100, Data: fullLine},
			ID:           sim.NewID(),
			Src:          cuPort.AsRemote(),
			Dst:          c.Ports.Top.AsRemote(),
			TrafficBytes: 64 + 12,
		}

		c.Ports.Top.Deliver(w)
		require.NoError(t, engine.Run())
		require.Len(t, drainResponses(), 1)

		reqInStart, ok := rec.firstStart("req_in")
		require.True(t, ok)

		// The bank access is a pipeline subtask (What ".bank") parented to the
		// req_in: every "work" interval has a corresponding child bar.
		var bankStart tasks.Task
		found := false
		for _, s := range rec.starts {
			if s.Kind == tasks.KindPipeline && s.What == "Cache.bank" {
				bankStart = s
				found = true
			}
		}
		require.True(t, found)
		require.Equal(t, reqInStart.ID, bankStart.ParentID)

		// The req_in still carries the matching work milestone for that bar.
		require.True(t, rec.hasMilestone(reqInStart.ID, "Cache.bank"))
		for _, m := range rec.milestonesOn(reqInStart.ID) {
			if m.What == "Cache.bank" {
				require.Equal(t, tasks.MilestoneKindWork, m.Kind)
			}
		}
	})
}
