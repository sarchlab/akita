package addresstranslator

import (
	"testing"

	"github.com/sarchlab/akita/v5/internal/testsim"
	"github.com/sarchlab/akita/v5/mem/memprotocol"
	"github.com/sarchlab/akita/v5/mem/vm"
	"github.com/sarchlab/akita/v5/mem/vm/vmprotocol"
	"github.com/sarchlab/akita/v5/sim/messaging"
	"github.com/sarchlab/akita/v5/sim/modeling"
	"github.com/sarchlab/akita/v5/sim/tasks"
	"github.com/sarchlab/akita/v5/sim/timing"
	"github.com/stretchr/testify/require"
)

// traceRecorder captures the full trace event stream the address translator
// emits — task starts/ends, milestones, and tags — in emission order and
// without the DBTracer's dedup, so a test can assert the exact ordered set of
// milestones and which req_out tasks are finalized.
type traceRecorder struct {
	tasks.NopObserver
	starts     []tasks.Task
	ends       []tasks.Task
	milestones []tasks.Milestone
	tags       []tasks.Tag
}

func (r *traceRecorder) TaskStarted(subject tasks.Task) {
	r.starts = append(r.starts, subject)
}

func (r *traceRecorder) TaskEnded(subject tasks.Task) {
	r.ends = append(r.ends, subject)
}

func (r *traceRecorder) MilestoneAdded(m tasks.Milestone) {
	r.milestones = append(r.milestones, m)
}

func (r *traceRecorder) TagAdded(subject tasks.Tag) {
	r.tags = append(r.tags, subject)
}

func (r *traceRecorder) endedIDs() []uint64 {
	ids := make([]uint64, len(r.ends))
	for i, e := range r.ends {
		ids[i] = e.ID
	}

	return ids
}

// taskID returns the ID of the first recorded task start of the given kind.
func (r *traceRecorder) taskID(kind string) uint64 {
	for _, s := range r.starts {
		if s.Kind == kind {
			return s.ID
		}
	}

	return 0
}

func (r *traceRecorder) milestonesOn(taskID uint64) []tasks.Milestone {
	var ms []tasks.Milestone
	for _, m := range r.milestones {
		if m.TaskID == taskID {
			ms = append(ms, m)
		}
	}

	return ms
}

func (r *traceRecorder) kindsOn(taskID uint64) []tasks.MilestoneKind {
	ms := r.milestonesOn(taskID)
	ks := make([]tasks.MilestoneKind, len(ms))
	for i, m := range ms {
		ks[i] = m.Kind
	}

	return ks
}

// milestoneClock lets manual middleware ticks observe nonzero waits without
// running the event queue, which would also admit or retire test traffic.
type milestoneClock struct {
	timing.Engine
	now timing.VTimeInPicoSec
}

func (c *milestoneClock) CurrentTime() timing.VTimeInPicoSec { return c.now }

func TestAddressTranslatorMilestones(t *testing.T) {
	var (
		engine          timing.Engine
		sim             timing.Simulation
		at              *Comp
		topPort         messaging.Port
		bottomPort      messaging.Port
		translationPort messaging.Port
		ptMW            *parseTranslateMW
		rpMW            *respondPipelineMW
		rec             *traceRecorder
	)

	setup1 := func(t *testing.T) {
		t.Helper()

		engine = &milestoneClock{Engine: timing.NewSerialEngine()}
		sim = testsim.New(engine, true)

		spec := DefaultSpec()
		spec.Log2PageSize = 12
		spec.Freq = 1

		spec.Bottom.Destinations = []messaging.RemotePort{messaging.RemotePort("MemPort")}
		spec.Translation.Destinations = []messaging.RemotePort{messaging.RemotePort("TranslationProvider")}

		at = modeling.MakeBuilder[*Comp](spec).
			WithSimulation(sim).
			Build("AddressTranslator")
		atPorts := makePorts("AddressTranslator", topBufSize)
		at.BindPort("Top", atPorts.Top)
		at.BindPort("Bottom", atPorts.Bottom)
		at.BindPort("Translation", atPorts.Translation)
		at.BindPort("Control", atPorts.Control)

		topPort = at.Ports.Top
		bottomPort = at.Ports.Bottom
		translationPort = at.Ports.Translation
		ctrlPort := at.Ports.Control

		for _, p := range []messaging.Port{
			topPort, bottomPort, translationPort, ctrlPort,
		} {
			conn := &noopConn{}
			conn.BindPort(p)
		}
		if err := sim.Initialize(); err != nil {
			panic(err)
		}

		ptMW = at.Middlewares.ParseTranslate
		rpMW = at.Middlewares.RespondPipeline

		// Attach the recorder before driving so request starts allocate
		// real receiver-side task IDs (it returns 0 when there are no hooks).
		rec = &traceRecorder{}
		at.TaskTracker().Subscribe(rec, at)
		// The translation-send admission milestone lands on the Top-port buffer
		// task, so the test needs the buffer task to exist.
	}

	makeRead := func(addr uint64) messaging.Msg {
		req := messaging.Msg{Payload: memprotocol.ReadReq{Address: addr, AccessByteSize: 4},
			ID:           sim.NewID(),
			Src:          messaging.RemotePort("Agent"),
			Dst:          topPort.AsRemote(),
			TrafficBytes: 12,
		}

		return req
	}

	makeWrite := func(addr uint64, data []byte) messaging.Msg {
		req := messaging.Msg{Payload: memprotocol.WriteReq{Address: addr, Data: data},
			ID:           sim.NewID(),
			Src:          messaging.RemotePort("Agent"),
			Dst:          topPort.AsRemote(),
			TrafficBytes: len(data) + 12,
		}

		return req
	}

	// driveRoundTrip pushes one request through the whole translator: admit
	// and send the translation (translate), receive the translation and
	// forward downstream (parseTranslation), then receive the bottom response
	// and reply upstream (respond). bottomRsp builds the bottom-side response
	// given the translated request's ID.
	driveRoundTrip := func(t *testing.T,
		req messaging.Msg,
		bottomRsp func(rspTo uint64) messaging.Msg,
	) {
		t.Helper()

		topPort.Deliver(req)
		engine.(*milestoneClock).now += 1000
		require.True(t, ptMW.translate())

		transReqID := at.State.Transactions[0].TranslationReqID
		translationPort.RetrieveOutgoing()

		transRsp := messaging.Msg{Payload: vmprotocol.TranslationRsp{
			Page: vm.Page{PID: 1, VAddr: 0x10000, PAddr: 0x20000},
		},
			ID:    sim.NewID(),
			RspTo: transReqID,
		}

		translationPort.Deliver(transRsp)

		engine.(*milestoneClock).now += 1000
		require.True(t, rpMW.parseTranslation())

		bottomReqID := at.State.InflightReqToBottom[0].ReqToBottomID
		bottomPort.RetrieveOutgoing()

		bottomPort.Deliver(bottomRsp(bottomReqID))

		engine.(*milestoneClock).now += 1000
		require.True(t, rpMW.respond())

		topPort.RetrieveOutgoing()
	}

	t.Run("splits admission (buffer task) from processing (req_in) for a read", func(t *testing.T) {
		setup1(t)

		driveRoundTrip(t, makeRead(0x10040), func(rspTo uint64) messaging.Msg {
			rsp := messaging.Msg{Payload: memprotocol.DataReadyRsp{Data: []byte{1, 2, 3, 4}},
				ID:    sim.NewID(),
				RspTo: rspTo,
			}

			return rsp
		})

		bufID := rec.taskID(tasks.KindIncomingBuffer)
		reqInID := rec.taskID("req_in")
		require.NotZero(t, bufID)
		require.NotZero(t, reqInID)
		require.NotEqual(t, reqInID, bufID)

		// The buffer task owns the pre-admission story: reached the head of the
		// Top buffer, then waited to send the translation request.
		require.Equal(t, []tasks.MilestoneKind{
			tasks.MilestoneKindNetworkBusy,
		}, rec.kindsOn(bufID))
		bufMs := rec.milestonesOn(bufID)

		require.Equal(t, translationPort.Name(), bufMs[0].What)

		// req_in owns only the post-admission processing — including the bottom
		// network-busy milestone (on the top req_in, not a phantom keyed by the
		// downstream request's receiver ID).
		require.Equal(t, []tasks.MilestoneKind{
			tasks.MilestoneKindTranslation,
			tasks.MilestoneKindNetworkBusy,
			tasks.MilestoneKindData,
			tasks.MilestoneKindNetworkBusy,
		}, rec.kindsOn(reqInID))
		reqMs := rec.milestonesOn(reqInID)
		require.Equal(t, "translation", reqMs[0].What)
		require.Equal(t, bottomPort.Name(), reqMs[1].What)
		require.Equal(t, "data", reqMs[2].What)
		require.Equal(t, topPort.Name(), reqMs[3].What)
	})

	t.Run("distinguishes a write with a subtask milestone on req_in", func(t *testing.T) {
		setup1(t)

		driveRoundTrip(t,
			makeWrite(0x10040, []byte{1, 2, 3, 4}),
			func(rspTo uint64) messaging.Msg {
				rsp := messaging.Msg{Payload: memprotocol.WriteDoneRsp{},
					ID:    sim.NewID(),
					RspTo: rspTo,
				}

				return rsp
			},
		)

		reqInID := rec.taskID("req_in")
		require.Equal(t, []tasks.MilestoneKind{
			tasks.MilestoneKindTranslation,
			tasks.MilestoneKindNetworkBusy,
			tasks.MilestoneKindSubTask,
			tasks.MilestoneKindNetworkBusy,
		}, rec.kindsOn(reqInID))
	})

	t.Run("finalizes the completing transaction's translation req_out, "+
		"not a later transaction shifted into its slot", func(t *testing.T) {
		setup1(t)

		// Two in-flight transactions; complete the first (non-last) one. The
		// removeTransaction append-shift must not redirect the trace finalize
		// to the surviving transaction shifted into the freed slot.
		transReq1ID := sim.NewID()
		transReq2ID := sim.NewID()
		transTask1ID := at.TaskTracker().StartReqOut(at, messaging.Msg{ID: transReq1ID}, 0)
		transTask2ID := at.TaskTracker().StartReqOut(at, messaging.Msg{ID: transReq2ID}, 0)

		req := makeRead(0x10040)

		at.State = state{
			Transactions: []transactionState{
				{
					TranslationReqID:  transReq1ID,
					TranslationTaskID: transTask1ID,
					TranslationReqSrc: translationPort.AsRemote(),
					TranslationReqDst: messaging.RemotePort("TranslationProvider"),
					IncomingReqs: []incomingReqState{
						msgToIncomingReqState(req),
					},
					TranslationDone: true,
				},
				{TranslationReqID: transReq2ID, TranslationTaskID: transTask2ID},
			},
		}

		transRsp := messaging.Msg{Payload: vmprotocol.TranslationRsp{
			Page: vm.Page{PID: 1, VAddr: 0x10000, PAddr: 0x20000},
		},
			ID:    sim.NewID(),
			RspTo: transReq1ID,
		}

		translationPort.Deliver(transRsp)

		engine.(*milestoneClock).now += 1000

		require.True(t, rpMW.parseTranslation())

		require.Contains(t, rec.endedIDs(), transTask1ID)
		require.NotContains(t, rec.endedIDs(), transTask2ID)
	})
}
