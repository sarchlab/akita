package simplebankedmemory

import (
	"fmt"
	"slices"

	"testing"

	"github.com/sarchlab/akita/v5/internal/testsim"
	"github.com/sarchlab/akita/v5/mem"
	"github.com/sarchlab/akita/v5/mem/memprotocol"
	"github.com/sarchlab/akita/v5/sim/messaging"
	"github.com/sarchlab/akita/v5/sim/modeling"
	"github.com/sarchlab/akita/v5/sim/modeling/modelingtest"
	"github.com/sarchlab/akita/v5/sim/tasks"
	"github.com/sarchlab/akita/v5/sim/timing"
	"github.com/stretchr/testify/require"
)

// milestoneRecorder captures the task starts and milestones the memory emits, in
// emission order and without the DBTracer's same-key/same-time dedup, so a test
// can assert the blocking reason and the task (buffer vs req_in) it is
// attributed to.
type milestoneRecorder struct {
	tasks.NopObserver
	starts     []tasks.Task
	ends       []tasks.Task
	milestones []tasks.Milestone
}

func (r *milestoneRecorder) TaskStarted(s tasks.Task) {
	r.starts = append(r.starts, s)
}

func (r *milestoneRecorder) TaskEnded(e tasks.Task) {
	r.ends = append(r.ends, e)
}

func (r *milestoneRecorder) MilestoneAdded(m tasks.Milestone) {
	r.milestones = append(r.milestones, m)
}

// taskID returns the ID of the first recorded task start of the given kind.
func (r *milestoneRecorder) taskID(kind string) uint64 {
	for _, s := range r.starts {
		if s.Kind == kind {
			return s.ID
		}
	}

	return 0
}

// taskStart returns the first recorded task start of the given kind and whether
// one was found.
func (r *milestoneRecorder) taskStart(kind string) (tasks.Task, bool) {
	for _, s := range r.starts {
		if s.Kind == kind {
			return s, true
		}
	}

	return tasks.Task{}, false
}

// ended reports whether a task with the given ID was ended.
func (r *milestoneRecorder) ended(taskID uint64) bool {
	for _, e := range r.ends {
		if e.ID == taskID {
			return true
		}
	}

	return false
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

// milestoneClock lets manual middleware ticks observe nonzero waits without
// running the event queue, which would also admit or retire test traffic.
type milestoneClock struct {
	timing.Engine
	now timing.VTimeInPicoSec
}

func (c *milestoneClock) CurrentTime() timing.VTimeInPicoSec { return c.now }

//nolint:funlen // Keep the existing scenarios and shared fixture lifecycle in one test group.
func TestSimpleBankedMemoryAdmissionMilestones(t *testing.T) {
	var (
		engine  timing.Engine
		sim     timing.Simulation
		memComp *Comp
		topPort messaging.Port
		rec     *milestoneRecorder
	)

	setup1 := func(t *testing.T) {
		t.Helper()

		engine = &milestoneClock{Engine: timing.NewSerialEngine()}
		sim = testsim.New(engine, true)
		storage := mem.NewStorage(4 * mem.GB)

		spec := DefaultSpec()
		spec.NumBanks = 2
		spec.StageLatency = 2

		memComp = modeling.MakeBuilder[*Comp](spec).
			WithSimulation(sim).
			WithResources(Resources{Storage: storage}).
			Build("Mem")
		memCompPorts := makePorts("Mem", 4, 16)
		memComp.BindPort("Top", memCompPorts.Top)
		memComp.BindPort("Control", memCompPorts.Control)
		if err := sim.Initialize(); err != nil {
			panic(err)
		}

		topPort = memComp.Ports.Top

		// Attach the recorder before driving so buffer enqueue allocates a
		// real buffer task ID (it returns 0 with no hooks). The admission
		// milestone lands on the Top-port buffer task, so the buffer task must
		// exist.
		rec = &milestoneRecorder{}
		memComp.TaskTracker().Subscribe(rec, memComp)
	}

	makeRead := func(addr uint64) messaging.Msg {
		req := messaging.Msg{Payload: memprotocol.ReadReq{Address: addr, AccessByteSize: 4},
			ID:           sim.NewID(),
			Src:          messaging.RemotePort("Agent"),
			Dst:          topPort.AsRemote(),
			TrafficBytes: 12,
			TrafficClass: "memprotocol.ReadReq"}

		return req
	}

	t.Run("records a per-bank hardware_resource admission milestone on "+
		"the buffer task", func(t *testing.T) {
		setup1(t)

		req := makeRead(0x0)
		topPort.Deliver(req)
		engine.(*milestoneClock).now += 1000

		spec := memComp.Spec
		bankID := selectBank(spec, bankSelectionAddress(spec, req.Payload.(memprotocol.ReadReq).Address))

		// One tick runs dispatchFromTopPort, which admits the request: the
		// buffer task's bank admission milestone is emitted just before
		// RetrieveIncoming.
		modelingtest.Tick(memComp)

		bufID := rec.taskID(tasks.KindIncomingBuffer)
		require.NotZero(t, bufID)

		bufMs := rec.milestonesOn(bufID)
		// No queue delay: the buffer task carries only the bank admission milestone.
		require.Len(t, bufMs, 1)

		require.Equal(t, tasks.MilestoneKindHardwareResource, bufMs[0].Kind)
		require.Equal(t, fmt.Sprintf("%s.bank%d", memComp.Name(), bankID), bufMs[0].What)
	})
}

//nolint:funlen // Keep the existing scenarios and shared fixture lifecycle in one test group.
func TestSimpleBankedMemoryPipelineTraversalMilestones(t *testing.T) {
	var (
		engine  timing.Engine
		sim     timing.Simulation
		storage *mem.Storage
		memComp *Comp
		topPort messaging.Port
		agent   *testAgent
		conn    *loopbackConnection
		rec     *milestoneRecorder
	)

	setup2 := func(t *testing.T) {
		t.Helper()

		engine = &milestoneClock{Engine: timing.NewSerialEngine()}
		sim = testsim.New(engine, true)
		storage = mem.NewStorage(4 * mem.GB)

		spec := DefaultSpec()
		spec.NumBanks = 2
		spec.StageLatency = 3

		memComp = modeling.MakeBuilder[*Comp](spec).
			WithSimulation(sim).
			WithResources(Resources{Storage: storage}).
			Build("Mem")
		memCompPorts := makePorts("Mem", 4, 16)
		memComp.BindPort("Top", memCompPorts.Top)
		memComp.BindPort("Control", memCompPorts.Control)

		topPort = memComp.Ports.Top
		agent = newTestAgent("Agent")
		conn = newLoopbackConnection("Conn")
		conn.BindPort(topPort)
		conn.BindPort(agent.port)
		if err := sim.Initialize(); err != nil {
			panic(err)
		}

		// Attach the recorder before driving so the receiver-side task IDs are
		// real (request starts return 0 with no hooks).
		rec = &milestoneRecorder{}
		memComp.TaskTracker().Subscribe(rec, memComp)
	}

	// drive ticks the memory and shuttles messages over the loopback until the
	// agent has received the response, or a tick budget is exhausted.
	drive := func() {
		for i := 0; i < 64 && len(agent.received) == 0; i++ {
			modelingtest.Tick(memComp)
			conn.transfer()
		}
	}

	t.Run("attributes the bank-pipeline traversal as work on the read "+
		"req_in", func(t *testing.T) {
		setup2(t)

		data := []byte{1, 2, 3, 4}
		storage.Write(0x40, data)

		read := messaging.Msg{Payload: memprotocol.ReadReq{Address: 0x40, AccessByteSize: 4},
			ID:           sim.NewID(),
			Src:          agent.port.AsRemote(),
			Dst:          topPort.AsRemote(),
			TrafficBytes: 12,
			TrafficClass: "memprotocol.ReadReq"}

		agent.send(t, read)
		conn.transfer()
		drive()

		require.Len(t, agent.received, 1)
		_, ok := agent.received[0].Payload.(memprotocol.DataReadyRsp)
		rsp := agent.received[0]
		require.True(t, ok)
		require.Equal(t, data, rsp.Payload.(memprotocol.DataReadyRsp).Data)

		reqInID := rec.taskID("req_in")
		require.NotZero(t, reqInID)

		// The req_in carries a work milestone marking the end of the
		// bank-pipeline traversal, named "<comp>.pipeline".
		reqInMs := rec.milestonesOn(reqInID)
		require.True(t, slices.ContainsFunc(reqInMs, func(m tasks.Milestone) bool {
			return m.Kind == tasks.MilestoneKindWork && m.What == memComp.Name()+".pipeline"
		}), "req_in must contain the bank-pipeline work milestone")

		// The pipeline subtask is a child of req_in and is closed at exit.
		pipeStart, found := rec.taskStart(tasks.KindPipeline)
		require.True(t, found)
		require.Equal(t, reqInID, pipeStart.ParentID)
		require.Equal(t, memComp.Name()+".pipeline", pipeStart.What)
		require.True(t, rec.ended(pipeStart.ID))
	})

	t.Run("attributes the bank-pipeline traversal as work on the write "+
		"req_in", func(t *testing.T) {
		setup2(t)

		write := messaging.Msg{Payload: memprotocol.WriteReq{
			Address: 0x80,
			Data:    []byte{9, 8, 7, 6},
		},
			ID:  sim.NewID(),
			Src: agent.port.AsRemote(),
			Dst: topPort.AsRemote()}

		write.TrafficBytes = len(write.Payload.(memprotocol.WriteReq).Data) + 12
		write.TrafficClass = "memprotocol.WriteReq"

		agent.send(t, write)
		conn.transfer()
		drive()

		require.Len(t, agent.received, 1)
		_, ok := agent.received[0].Payload.(memprotocol.WriteDoneRsp)
		require.True(t, ok)

		reqInID := rec.taskID("req_in")
		require.NotZero(t, reqInID)

		reqInMs := rec.milestonesOn(reqInID)
		require.True(t, slices.ContainsFunc(reqInMs, func(m tasks.Milestone) bool {
			return m.Kind == tasks.MilestoneKindWork && m.What == memComp.Name()+".pipeline"
		}), "req_in must contain the bank-pipeline work milestone")

		pipeStart, found := rec.taskStart(tasks.KindPipeline)
		require.True(t, found)
		require.Equal(t, reqInID, pipeStart.ParentID)
		require.True(t, rec.ended(pipeStart.ID))
	})
}
