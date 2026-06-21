package tracing

import (
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/sarchlab/akita/v5/hooking"
	"github.com/sarchlab/akita/v5/messaging"
	"github.com/sarchlab/akita/v5/timing"
)

// ibFakeComp is a minimal component that satisfies both messaging.Component
// (so it can own a real port) and NamedHookable (so the tracing API can emit
// tasks on it). InvokeHook is provided by the embedded HookableBase, which is
// how CollectTrace forwards events to a tracer.
type ibFakeComp struct {
	hooking.HookableBase
	name string
	time timing.VTimeInPicoSec
}

func (c *ibFakeComp) Name() string                       { return c.name }
func (c *ibFakeComp) CurrentTime() timing.VTimeInPicoSec { return c.time }

func (c *ibFakeComp) DeclarePort(string, ...*messaging.Role) {}
func (c *ibFakeComp) AssignPort(string, messaging.Port)      {}
func (c *ibFakeComp) GetPortByName(string) messaging.Port    { return nil }
func (c *ibFakeComp) Ports() []messaging.Port                { return nil }
func (c *ibFakeComp) NotifyRecv(messaging.Port)              {}
func (c *ibFakeComp) NotifyPortFree(messaging.Port)          {}

// ibRecordingTracer captures the task events the hook produces.
type ibRecordingTracer struct {
	NopTracer
	starts     []TaskStart
	ends       []TaskEnd
	milestones []Milestone
}

func (t *ibRecordingTracer) StartTask(ts TaskStart)   { t.starts = append(t.starts, ts) }
func (t *ibRecordingTracer) EndTask(te TaskEnd)       { t.ends = append(t.ends, te) }
func (t *ibRecordingTracer) AddMilestone(m Milestone) { t.milestones = append(t.milestones, m) }

type ibTestMsg struct {
	messaging.MsgMeta
}

var _ = Describe("Incoming buffer tracer", func() {
	var (
		comp   *ibFakeComp
		tracer *ibRecordingTracer
		port   messaging.Port
	)

	BeforeEach(func() {
		comp = &ibFakeComp{name: "Comp"}
		tracer = &ibRecordingTracer{}
		CollectTrace(comp, tracer)

		port = messaging.NewPort(comp, 4, 4, "Comp.Top")
		CollectIncomingBufferTrace(port)
	})

	It("spans delivery to retrieve and marks reached-head the instant the "+
		"message becomes the head of the buffer", func() {
		// A lands in an empty buffer => immediately at the head.
		a := ibTestMsg{messaging.MsgMeta{ID: 7, Dst: "Comp.Top"}}
		comp.time = 100
		port.Deliver(a)

		// B lands behind A => not yet at the head.
		b := ibTestMsg{messaging.MsgMeta{ID: 8, Dst: "Comp.Top"}}
		comp.time = 110
		port.Deliver(b)

		Expect(tracer.starts).To(HaveLen(2))
		startA := tracer.starts[0]
		Expect(startA.Kind).To(Equal(IncomingBufferTaskKind))
		Expect(startA.ParentID).To(Equal(uint64(7)))
		Expect(startA.What).To(Equal("ibTestMsg"))
		Expect(startA.Location).To(Equal("Comp.Top"))
		Expect(startA.Time).To(Equal(timing.VTimeInPicoSec(100)))
		startB := tracer.starts[1]
		Expect(startB.ParentID).To(Equal(uint64(8)))
		Expect(startB.Time).To(Equal(timing.VTimeInPicoSec(110)))

		// A reached the head at delivery (empty buffer); B has not.
		Expect(tracer.milestones).To(HaveLen(1))
		Expect(tracer.milestones[0].TaskID).To(Equal(startA.ID))
		Expect(tracer.milestones[0].Kind).To(Equal(MilestoneKindQueue))
		Expect(tracer.milestones[0].What).To(Equal("Comp.Top"))
		Expect(tracer.milestones[0].Time).To(Equal(timing.VTimeInPicoSec(100)))

		// Neither task ends until its own message is retrieved.
		Expect(tracer.ends).To(BeEmpty())

		// Retrieving A ends A's buffer task and exposes B at the head.
		comp.time = 150
		Expect(port.RetrieveIncoming()).NotTo(BeNil())

		Expect(tracer.ends).To(HaveLen(1))
		Expect(tracer.ends[0].ID).To(Equal(startA.ID))
		Expect(tracer.ends[0].Time).To(Equal(timing.VTimeInPicoSec(150)))

		Expect(tracer.milestones).To(HaveLen(2))
		Expect(tracer.milestones[1].TaskID).To(Equal(startB.ID))
		Expect(tracer.milestones[1].Kind).To(Equal(MilestoneKindQueue))
		Expect(tracer.milestones[1].Time).To(Equal(timing.VTimeInPicoSec(150)))

		// Retrieving B ends B's buffer task.
		comp.time = 160
		Expect(port.RetrieveIncoming()).NotTo(BeNil())
		Expect(tracer.ends).To(HaveLen(2))
		Expect(tracer.ends[1].ID).To(Equal(startB.ID))
		Expect(tracer.ends[1].Time).To(Equal(timing.VTimeInPicoSec(160)))
	})

	It("parents a response's buffer task to the task it responds to", func() {
		rsp := ibTestMsg{messaging.MsgMeta{ID: 9, RspTo: 7, Dst: "Comp.Top"}}

		comp.time = 200
		port.Deliver(rsp)

		Expect(tracer.starts).To(HaveLen(1))
		Expect(tracer.starts[0].ParentID).To(Equal(uint64(7)))
	})

	It("is a no-op when the owning component is not being traced", func() {
		untraced := &ibFakeComp{name: "Untraced"}
		p2 := messaging.NewPort(untraced, 4, 4, "Untraced.Top")
		CollectIncomingBufferTrace(p2)

		untraced.time = 100
		p2.Deliver(ibTestMsg{messaging.MsgMeta{ID: 1, Dst: "Untraced.Top"}})
		Expect(p2.RetrieveIncoming()).NotTo(BeNil())

		Expect(tracer.starts).To(BeEmpty())
		Expect(tracer.ends).To(BeEmpty())
		Expect(tracer.milestones).To(BeEmpty())
	})
})
