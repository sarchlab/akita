package writeback

import (
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/sarchlab/akita/v5/mem"
	"github.com/sarchlab/akita/v5/mem/memprotocol"
	"github.com/sarchlab/akita/v5/sim/messaging"
	"github.com/sarchlab/akita/v5/sim/messaging/direct"
	"github.com/sarchlab/akita/v5/sim/modeling"
	"github.com/sarchlab/akita/v5/sim/timing"
	"github.com/sarchlab/akita/v5/sim/tracing"
)

// wbMilestoneRecorder captures the task starts, milestones, and tags the
// writeback cache emits, in emission order and without the DBTracer's
// same-key/same-time dedup, so a test can assert the buffer-vs-req_in/pipeline
// task each milestone and tag is attributed to.
type wbMilestoneRecorder struct {
	tracing.NopTracer
	starts     []tracing.TaskStart
	milestones []tracing.Milestone
	tags       []tracing.TaskTag
}

func (r *wbMilestoneRecorder) StartTask(s tracing.TaskStart) {
	r.starts = append(r.starts, s)
}

func (r *wbMilestoneRecorder) AddMilestone(m tracing.Milestone) {
	r.milestones = append(r.milestones, m)
}

func (r *wbMilestoneRecorder) AddTaskTag(t tracing.TaskTag) {
	r.tags = append(r.tags, t)
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
func (r *wbMilestoneRecorder) startsOfKind(kind string) []tracing.TaskStart {
	var out []tracing.TaskStart
	for _, s := range r.starts {
		if s.Kind == kind {
			out = append(out, s)
		}
	}

	return out
}

func (r *wbMilestoneRecorder) milestonesOn(taskID uint64) []tracing.Milestone {
	var ms []tracing.Milestone
	for _, m := range r.milestones {
		if m.TaskID == taskID {
			ms = append(ms, m)
		}
	}

	return ms
}

var _ = Describe("Write-Back Cache milestones", func() {
	var (
		engine      timing.Engine
		sim         timing.Simulation
		cacheComp   *Comp
		dramTop     messaging.Port
		dramStorage *mem.Storage
		conn        *direct.Connection
		agentPort   messaging.Port
		topPort     messaging.Port
		rec         *wbMilestoneRecorder
	)

	BeforeEach(func() {
		engine = timing.NewSerialEngine()
		sim = modeling.NewStandaloneSimulation(engine)

		agentPort = newDriverPort("Agent.Top", 8)

		dramStorage = mem.NewStorage(4 * mem.GB)
		dramTop = buildIdealDRAM(sim, dramStorage)

		addressToPortMapper := &mem.SinglePortMapper{
			Port: dramTop.AsRemote(),
		}

		cacheSpec := Definition.DefaultSpec
		cacheSpec.TotalByteSize = 1024 * 4 * 64
		cacheSpec.NumReqPerCycle = 4

		cacheComp = Definition.Builder().
			WithSimulation(sim).
			WithSpec(cacheSpec).
			WithResources(Resources{
				Storage:             mem.NewStorage(cacheSpec.TotalByteSize),
				AddressToPortMapper: addressToPortMapper,
			}).
			WithPorts(makePorts("Cache", 8)).
			Build("Cache")
		topPort = cacheComp.Ports.Top

		conn = direct.NewConnection("Connection", sim, timing.GHz)
		conn.PlugIn(topPort)
		conn.PlugIn(cacheComp.Ports.Bottom)
		conn.PlugIn(cacheComp.Ports.Control)
		conn.PlugIn(dramTop)
		conn.PlugIn(agentPort)

		// Attach the recorder before driving so MsgIDAtReceiver hands out real
		// receiver-side task IDs (it returns 0 when there are no hooks). The
		// admission milestone lands on the Top-port buffer task, so the test
		// also needs the incoming-buffer task to exist.
		rec = &wbMilestoneRecorder{}
		tracing.CollectTrace(cacheComp, rec)
		tracing.CollectIncomingBufferTrace(topPort)
	})

	It("records the dir-stage-buf admission milestone on the buffer task "+
		"and a directory-pipeline subtask parented to req_in for a read miss",
		func() {
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
				TrafficClass: "memprotocol.ReadReq"}

			topPort.Deliver(read)

			Expect(engine.Run()).To(Succeed())

			rsp, _ := agentPort.RetrieveIncoming()
			dr := rsp
			Expect(dr.Payload.(memprotocol.DataReadyRsp).Data).To(Equal([]byte{5, 6, 7, 8}))
			Expect(dr.RspTo).To(Equal(read.ID))

			bufID := rec.taskID(tracing.IncomingBufferTaskKind)
			reqInID := rec.taskID("req_in")
			Expect(bufID).ToNot(BeZero())
			Expect(reqInID).ToNot(BeZero())
			Expect(bufID).ToNot(Equal(reqInID))

			// The admission milestone (left the Top buffer because the directory
			// stage buffer had room) lands on the incoming-buffer task, after the
			// queue reached-head milestone the port hook emits.
			found := false
			for _, m := range rec.milestonesOn(bufID) {
				if m.Kind == tracing.MilestoneKindHardwareResource {
					Expect(m.What).To(Equal(cacheComp.Name() + ".dir_stage_buf"))
					found = true
				}
			}
			Expect(found).To(BeTrue(),
				"expected a hardware_resource admission milestone on the buffer task")

			// A directory-pipeline subtask exists and is parented to the
			// request's req_in task, closing the retrieve->directory gap.
			pipeStarts := rec.startsOfKind(tracing.PipelineTaskKind)
			Expect(pipeStarts).ToNot(BeEmpty())
			Expect(pipeStarts[0].ParentID).To(Equal(reqInID))
			Expect(pipeStarts[0].What).To(Equal(cacheComp.Name() + ".dir_pipeline"))
		})

	It("records the dir-pipeline work milestone and the downstream-fetch "+
		"data milestone on req_in for a read miss",
		func() {
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
				TrafficClass: "memprotocol.ReadReq"}

			topPort.Deliver(read)

			Expect(engine.Run()).To(Succeed())

			// Drive the read miss all the way through the downstream fetch
			// response: the agent must have received the data.
			rsp, _ := agentPort.RetrieveIncoming()
			dr := rsp
			Expect(dr.Payload.(memprotocol.DataReadyRsp).Data).To(Equal([]byte{5, 6, 7, 8}))
			Expect(dr.RspTo).To(Equal(read.ID))

			reqInID := rec.taskID("req_in")
			Expect(reqInID).ToNot(BeZero())

			ms := rec.milestonesOn(reqInID)

			// (a) The directory-pipeline-exit work milestone lands on req_in,
			// marking the post-pipeline processing interval as work.
			haveWork := false
			for _, m := range ms {
				if m.Kind == tracing.MilestoneKindWork &&
					m.What == cacheComp.Name()+".dir_pipeline" {
					haveWork = true
				}
			}
			Expect(haveWork).To(BeTrue(),
				"expected a work/.dir_pipeline milestone on req_in")

			// (b) The downstream fetch response charges a data milestone on
			// req_in, marking the data-wait resolved at fetch-response arrival.
			haveData := false
			for _, m := range ms {
				if m.Kind == tracing.MilestoneKindData &&
					m.What == cacheComp.Name()+".Bottom" {
					haveData = true
				}
			}
			Expect(haveData).To(BeTrue(),
				"expected a data/.Bottom milestone on req_in after the fetch response")
		})
})
