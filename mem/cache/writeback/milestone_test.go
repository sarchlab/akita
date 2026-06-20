package writeback

import (
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/sarchlab/akita/v5/mem"
	"github.com/sarchlab/akita/v5/mem/idealmemcontroller"
	"github.com/sarchlab/akita/v5/mem/memprotocol"
	"github.com/sarchlab/akita/v5/messaging"
	"github.com/sarchlab/akita/v5/modeling"
	"github.com/sarchlab/akita/v5/noc/directconnection"
	"github.com/sarchlab/akita/v5/timing"
	"github.com/sarchlab/akita/v5/tracing"
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
		cacheComp   *Comp
		dram        *idealmemcontroller.Comp
		dramStorage *mem.Storage
		conn        *directconnection.Comp
		agentPort   messaging.Port
		topPort     messaging.Port
		rec         *wbMilestoneRecorder
	)

	BeforeEach(func() {
		engine = timing.NewSerialEngine()

		agentPort = messaging.NewPort(nil, 8, 8, "Agent.Top")

		dramStorage = mem.NewStorage(4 * mem.GB)
		dramSpec := idealmemcontroller.DefaultSpec()
		dramSpec.Width = 1
		dramSpec.Latency = 200
		dramSpec.CacheLineSize = 64
		dram = idealmemcontroller.MakeBuilder().
			WithRegistrar(modeling.NewStandaloneRegistrar(engine)).
			WithResources(idealmemcontroller.Resources{Storage: dramStorage}).
			WithSpec(dramSpec).
			Build("DRAM")
		dram.AssignPort("Top",
			messaging.NewPort(dram, 16, 16, dram.Name()+".Top"))
		dram.AssignPort("Control",
			messaging.NewPort(dram, 16, 16, dram.Name()+".Control"))

		addressToPortMapper := &mem.SinglePortMapper{
			Port: dram.GetPortByName("Top").AsRemote(),
		}

		cacheSpec := DefaultSpec()
		cacheSpec.TotalByteSize = 1024 * 4 * 64
		cacheSpec.NumReqPerCycle = 4

		cacheComp = MakeBuilder().
			WithRegistrar(modeling.NewStandaloneRegistrar(engine)).
			WithSpec(cacheSpec).
			WithResources(Resources{
				AddressToPortMapper: addressToPortMapper,
			}).
			Build("Cache")
		for _, name := range []string{"Top", "Bottom", "Control"} {
			cacheComp.AssignPort(name,
				messaging.NewPort(cacheComp, 8, 8, cacheComp.Name()+"."+name))
		}
		topPort = cacheComp.GetPortByName("Top")

		conn = directconnection.MakeBuilder().
			WithRegistrar(modeling.NewStandaloneRegistrar(engine)).
			Build("Connection")
		conn.PlugIn(topPort)
		conn.PlugIn(cacheComp.GetPortByName("Bottom"))
		conn.PlugIn(cacheComp.GetPortByName("Control"))
		conn.PlugIn(dram.GetPortByName("Top"))
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

			read := memprotocol.ReadReq{}
			read.ID = timing.GetIDGenerator().Generate()
			read.Src = agentPort.AsRemote()
			read.Dst = topPort.AsRemote()
			read.Address = 0x10004
			read.AccessByteSize = 4
			read.TrafficBytes = 12
			read.TrafficClass = "memprotocol.ReadReq"
			topPort.Deliver(read)

			engine.Run()

			rsp := agentPort.RetrieveIncoming()
			dr := rsp.(memprotocol.DataReadyRsp)
			Expect(dr.Data).To(Equal([]byte{5, 6, 7, 8}))
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
})
