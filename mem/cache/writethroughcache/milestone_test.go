package writethroughcache_test

import (
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/sarchlab/akita/v5/mem"
	. "github.com/sarchlab/akita/v5/mem/cache/writethroughcache"
	"github.com/sarchlab/akita/v5/mem/idealmemcontroller"
	"github.com/sarchlab/akita/v5/mem/memprotocol"
	"github.com/sarchlab/akita/v5/messaging"
	"github.com/sarchlab/akita/v5/modeling"
	"github.com/sarchlab/akita/v5/noc/directconnection"
	"github.com/sarchlab/akita/v5/timing"
	"github.com/sarchlab/akita/v5/tracing"
)

// milestoneRecorder captures the task starts, milestones, and tags the cache
// emits, in emission order and without any same-key/same-time dedup, so a test
// can assert the full ordered set of blocking reasons and the task (buffer vs
// req_in vs pipeline subtask) each one is attributed to. Mirrors the recorder
// used in mem/rob/milestone_test.go.
type milestoneRecorder struct {
	tracing.NopTracer
	starts     []tracing.TaskStart
	milestones []tracing.Milestone
	tags       []tracing.TaskTag
}

func (r *milestoneRecorder) StartTask(s tracing.TaskStart) {
	r.starts = append(r.starts, s)
}

func (r *milestoneRecorder) AddMilestone(m tracing.Milestone) {
	r.milestones = append(r.milestones, m)
}

func (r *milestoneRecorder) AddTaskTag(t tracing.TaskTag) {
	r.tags = append(r.tags, t)
}

// firstStart returns the first recorded task start of the given kind.
func (r *milestoneRecorder) firstStart(kind string) (tracing.TaskStart, bool) {
	for _, s := range r.starts {
		if s.Kind == kind {
			return s, true
		}
	}

	return tracing.TaskStart{}, false
}

func (r *milestoneRecorder) milestonesOn(taskID uint64) []tracing.Milestone {
	var ms []tracing.Milestone
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

func (r *milestoneRecorder) tagsWith(what string) []tracing.TaskTag {
	var ts []tracing.TaskTag
	for _, t := range r.tags {
		if t.What == what {
			ts = append(ts, t)
		}
	}

	return ts
}

var _ = Describe("Cache milestones", func() {
	var (
		engine      timing.Engine
		connection  messaging.Connection
		dram        *idealmemcontroller.Comp
		dramStorage *mem.Storage
		cuPort      messaging.Port
		c           *modeling.Component[Spec, State, Resources]
		rec         *milestoneRecorder
	)

	buildCache := func(policy string) {
		engine = timing.NewSerialEngine()
		connection = directconnection.MakeBuilder().
			WithRegistrar(modeling.NewStandaloneRegistrar(engine)).
			Build("Conn")

		cuPort = messaging.NewPort(nil, 16, 16, "CU.Top")

		dramStorage = mem.NewStorage(4 * mem.GB)
		dram = idealmemcontroller.MakeBuilder().
			WithRegistrar(modeling.NewStandaloneRegistrar(engine)).
			WithResources(idealmemcontroller.Resources{Storage: dramStorage}).
			Build("DRAM")
		dram.AssignPort("Top",
			messaging.NewPort(dram, 16, 16, dram.Name()+".Top"))
		dram.AssignPort("Control",
			messaging.NewPort(dram, 16, 16, dram.Name()+".Control"))
		addressToPortMapper := &mem.SinglePortMapper{
			Port: dram.GetPortByName("Top").AsRemote(),
		}

		cacheReg := modeling.NewStandaloneRegistrar(engine)
		spec := DefaultSpec()
		spec.WritePolicyType = policy
		c = MakeBuilder().
			WithRegistrar(cacheReg).
			WithSpec(spec).
			WithResources(Resources{AddressMapper: addressToPortMapper}).
			Build("Cache")

		for _, name := range []string{"Top", "Bottom", "Control"} {
			p := modeling.MakePortBuilder().
				WithRegistrar(cacheReg).
				WithComponent(c).
				WithSpec(modeling.PortSpec{BufSize: 4}).
				Build(name)
			c.AssignPort(name, p)
		}

		connection.PlugIn(dram.GetPortByName("Top"))
		connection.PlugIn(c.GetPortByName("Top"))
		connection.PlugIn(c.GetPortByName("Bottom"))
		connection.PlugIn(cuPort)

		rec = &milestoneRecorder{}
		tracing.CollectTrace(c, rec)
		// The admission milestones and the buffer task land on the Top-port
		// buffer task, so the test needs that task to exist.
		tracing.CollectIncomingBufferTrace(c.GetPortByName("Top"))
	}

	drainResponses := func() []messaging.Msg {
		msgs := []messaging.Msg{}
		for {
			msg := cuPort.RetrieveIncoming()
			if msg == nil {
				break
			}
			msgs = append(msgs, msg)
		}
		return msgs
	}

	It("records admission milestones on the buffer task and a directory "+
		"pipeline subtask parented to req_in for a read miss", func() {
		buildCache("write-around")

		dramStorage.Write(0x100, []byte{1, 2, 3, 4})
		read := memprotocol.ReadReq{Address: 0x100, AccessByteSize: 4}
		read.ID = timing.GetIDGenerator().Generate()
		read.Src = cuPort.AsRemote()
		read.Dst = c.GetPortByName("Top").AsRemote()
		read.TrafficBytes = 12
		read.TrafficClass = "req"
		c.GetPortByName("Top").Deliver(read)

		engine.Run()

		rsps := drainResponses()
		Expect(rsps).To(HaveLen(1))
		Expect(rsps[0].(memprotocol.DataReadyRsp).Data).
			To(Equal([]byte{1, 2, 3, 4}))

		// The buffer task exists and carries the two hardware-resource
		// admission milestones the intake emits before retrieve.
		bufStart, ok := rec.firstStart(tracing.IncomingBufferTaskKind)
		Expect(ok).To(BeTrue())
		Expect(rec.hasMilestone(bufStart.ID, "Cache.trans")).To(BeTrue())
		Expect(rec.hasMilestone(bufStart.ID, "Cache.dir_buf")).To(BeTrue())

		// req_in is the cache's receiver-side task for the read request. Its
		// ID comes from the recorded task start; recomputing it via
		// MsgIDAtReceiver post-run would mint a fresh ID, because the registry
		// entry is released when the request completes.
		reqInStart, ok := rec.firstStart("req_in")
		Expect(ok).To(BeTrue())
		Expect(reqInStart.ID).ToNot(BeZero())

		// The directory pipeline subtask exists and is parented to req_in
		// (not to the cache_transaction task), per the buffer-task convention.
		pipeStart, ok := rec.firstStart(tracing.PipelineTaskKind)
		Expect(ok).To(BeTrue())
		Expect(pipeStart.What).To(Equal("Cache.dir_pipeline"))
		Expect(pipeStart.ParentID).To(Equal(reqInStart.ID))

		// The cache_transaction task is distinct from the pipeline subtask.
		ctStart, ok := rec.firstStart("cache_transaction")
		Expect(ok).To(BeTrue())
		Expect(pipeStart.ParentID).ToNot(Equal(ctStart.ID))
	})

	It("records the dir_pipeline work and bottom data milestones on req_in "+
		"for a read miss driven through the downstream fetch response", func() {
		buildCache("write-around")

		dramStorage.Write(0x100, []byte{1, 2, 3, 4})
		read := memprotocol.ReadReq{Address: 0x100, AccessByteSize: 4}
		read.ID = timing.GetIDGenerator().Generate()
		read.Src = cuPort.AsRemote()
		read.Dst = c.GetPortByName("Top").AsRemote()
		read.TrafficBytes = 12
		read.TrafficClass = "req"
		c.GetPortByName("Top").Deliver(read)

		// Run to quiescence: the read miss fetches from the lower memory and the
		// DataReadyRsp comes back, filling the line and completing the request.
		engine.Run()

		rsps := drainResponses()
		Expect(rsps).To(HaveLen(1))
		Expect(rsps[0].(memprotocol.DataReadyRsp).Data).
			To(Equal([]byte{1, 2, 3, 4}))

		reqInStart, ok := rec.firstStart("req_in")
		Expect(ok).To(BeTrue())
		Expect(reqInStart.ID).ToNot(BeZero())

		// WORK milestone: emitted on the req_in at the directory pipeline exit,
		// before the lookup's same-tick processing milestones.
		Expect(rec.hasMilestone(reqInStart.ID, "Cache.dir_pipeline")).
			To(BeTrue())
		for _, m := range rec.milestonesOn(reqInStart.ID) {
			if m.What == "Cache.dir_pipeline" {
				Expect(m.Kind).To(Equal(tracing.MilestoneKindWork))
			}
		}

		// DATA milestone: emitted on the req_in when the downstream read fill
		// (DataReadyRsp) arrives, keyed to the req_in, not the response.
		Expect(rec.hasMilestone(reqInStart.ID, "Cache.Bottom")).To(BeTrue())
		for _, m := range rec.milestonesOn(reqInStart.ID) {
			if m.What == "Cache.Bottom" {
				Expect(m.Kind).To(Equal(tracing.MilestoneKindData))
			}
		}
	})

	It("tags a write-through write hit with write-hit", func() {
		buildCache("write-through")

		// Prime the line into the cache with a full-line write so the
		// follow-up partial write lands on a valid block (a write hit).
		fullLine := make([]byte, 64)
		for i := range fullLine {
			fullLine[i] = byte(i)
		}
		warm := memprotocol.WriteReq{Address: 0x100, Data: fullLine}
		warm.ID = timing.GetIDGenerator().Generate()
		warm.Src = cuPort.AsRemote()
		warm.Dst = c.GetPortByName("Top").AsRemote()
		warm.TrafficBytes = 64 + 12
		warm.TrafficClass = "req"
		c.GetPortByName("Top").Deliver(warm)
		engine.Run()
		Expect(drainResponses()).To(HaveLen(1))

		// Discard the warm write's trace events so the assertions below see only
		// the partial-write-hit transaction.
		rec.starts = nil
		rec.milestones = nil
		rec.tags = nil

		// Now a partial write to the same line: this is a write-through write
		// hit and must carry the write-hit tag.
		hit := memprotocol.WriteReq{Address: 0x100, Data: []byte{9, 9, 9, 9}}
		hit.ID = timing.GetIDGenerator().Generate()
		hit.Src = cuPort.AsRemote()
		hit.Dst = c.GetPortByName("Top").AsRemote()
		hit.TrafficBytes = 4 + 12
		hit.TrafficClass = "req"
		c.GetPortByName("Top").Deliver(hit)
		engine.Run()
		Expect(drainResponses()).To(HaveLen(1))

		// The partial write hit opened exactly one cache_transaction; the
		// write-hit tag the directory now emits on writethroughWriteHit must
		// hang off that transaction.
		ctStart, ok := rec.firstStart("cache_transaction")
		Expect(ok).To(BeTrue())
		Expect(ctStart.What).To(Equal("write"))

		writeHitTags := rec.tagsWith("write-hit")
		Expect(writeHitTags).ToNot(BeEmpty())

		found := false
		for _, t := range writeHitTags {
			if t.TaskID == ctStart.ID {
				found = true
			}
		}
		Expect(found).To(BeTrue())

		// The write-through write dispatched a downstream WriteReq whose
		// WriteDoneRsp came back during this run. processDoneRsp records a
		// dependency milestone on the write's req_in (not the ack).
		reqInStart, ok := rec.firstStart("req_in")
		Expect(ok).To(BeTrue())
		Expect(rec.hasMilestone(reqInStart.ID, "Cache.Bottom")).To(BeTrue())
		for _, m := range rec.milestonesOn(reqInStart.ID) {
			if m.What == "Cache.Bottom" {
				Expect(m.Kind).To(Equal(tracing.MilestoneKindDependency))
			}
		}
	})
})
