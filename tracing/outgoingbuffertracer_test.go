package tracing

import (
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/sarchlab/akita/v5/hooking"
	"github.com/sarchlab/akita/v5/messaging"
	"github.com/sarchlab/akita/v5/timing"
)

// obFakeComp is a minimal component that satisfies both messaging.Component
// (so it can own a real port) and NamedHookable (so the tracing API can emit
// tasks on it). InvokeHook is provided by the embedded HookableBase, which is
// how CollectTrace forwards events to a tracer.
type obFakeComp struct {
	hooking.HookableBase
	name string
	time timing.VTimeInPicoSec
}

func (c *obFakeComp) Name() string                       { return c.name }
func (c *obFakeComp) CurrentTime() timing.VTimeInPicoSec { return c.time }

func (c *obFakeComp) DeclarePort(string, ...*messaging.Role) {}
func (c *obFakeComp) AssignPort(string, messaging.Port)      {}
func (c *obFakeComp) GetPortByName(string) messaging.Port    { return nil }
func (c *obFakeComp) Ports() []messaging.Port                { return nil }
func (c *obFakeComp) NotifyRecv(messaging.Port)              {}
func (c *obFakeComp) NotifyPortFree(messaging.Port)          {}

// obRecordingTracer captures the task events the hook produces.
type obRecordingTracer struct {
	NopTracer
	starts     []TaskStart
	ends       []TaskEnd
	milestones []Milestone
}

func (t *obRecordingTracer) StartTask(ts TaskStart)   { t.starts = append(t.starts, ts) }
func (t *obRecordingTracer) EndTask(te TaskEnd)       { t.ends = append(t.ends, te) }
func (t *obRecordingTracer) AddMilestone(m Milestone) { t.milestones = append(t.milestones, m) }

type obTestMsg struct {
	messaging.MsgMeta
}

// obFakeConn is a no-op connection so that Send into an empty outgoing buffer
// (which notifies the connection) does not dereference a nil pointer.
type obFakeConn struct {
	hooking.HookableBase
}

func (c *obFakeConn) Name() string                  { return "Conn" }
func (c *obFakeConn) PlugIn(messaging.Port)          {}
func (c *obFakeConn) Unplug(messaging.Port)          {}
func (c *obFakeConn) NotifyAvailable(messaging.Port) {}
func (c *obFakeConn) NotifySend()                    {}

var _ = Describe("Outgoing buffer tracer", func() {
	var (
		comp   *obFakeComp
		tracer *obRecordingTracer
		port   messaging.Port
	)

	BeforeEach(func() {
		comp = &obFakeComp{name: "Comp"}
		tracer = &obRecordingTracer{}
		CollectTrace(comp, tracer)

		port = messaging.NewPort(comp, 4, 4, "Comp.Bottom")
		port.SetConnection(&obFakeConn{})
		CollectOutgoingBufferTrace(port)
	})

	It("spans send to drain and marks reached-head the instant the message "+
		"becomes the head of the buffer", func() {
		// A enters an empty buffer => immediately at the head.
		a := obTestMsg{messaging.MsgMeta{ID: 7, Src: "Comp.Bottom", Dst: "Other.Top"}}
		comp.time = 100
		port.Send(a)

		// B enters behind A => not yet at the head.
		b := obTestMsg{messaging.MsgMeta{ID: 8, Src: "Comp.Bottom", Dst: "Other.Top"}}
		comp.time = 110
		port.Send(b)

		Expect(tracer.starts).To(HaveLen(2))
		startA := tracer.starts[0]
		Expect(startA.Kind).To(Equal(OutgoingBufferTaskKind))
		Expect(startA.ParentID).To(Equal(uint64(7)))
		Expect(startA.What).To(Equal("obTestMsg"))
		Expect(startA.Location).To(Equal("Comp.Bottom.outgoing"))
		Expect(startA.Time).To(Equal(timing.VTimeInPicoSec(100)))
		startB := tracer.starts[1]
		Expect(startB.ParentID).To(Equal(uint64(8)))
		Expect(startB.Time).To(Equal(timing.VTimeInPicoSec(110)))

		// A reached the head at send (empty buffer); B has not.
		Expect(tracer.milestones).To(HaveLen(1))
		Expect(tracer.milestones[0].TaskID).To(Equal(startA.ID))
		Expect(tracer.milestones[0].Kind).To(Equal(MilestoneKindQueue))
		Expect(tracer.milestones[0].What).To(Equal("Comp.Bottom"))
		Expect(tracer.milestones[0].Time).To(Equal(timing.VTimeInPicoSec(100)))

		// Neither task ends until its own message is drained.
		Expect(tracer.ends).To(BeEmpty())

		// Draining A ends A's buffer task and exposes B at the head.
		comp.time = 150
		Expect(port.RetrieveOutgoing()).NotTo(BeNil())

		Expect(tracer.ends).To(HaveLen(1))
		Expect(tracer.ends[0].ID).To(Equal(startA.ID))
		Expect(tracer.ends[0].Time).To(Equal(timing.VTimeInPicoSec(150)))

		Expect(tracer.milestones).To(HaveLen(2))
		Expect(tracer.milestones[1].TaskID).To(Equal(startB.ID))
		Expect(tracer.milestones[1].Kind).To(Equal(MilestoneKindQueue))
		Expect(tracer.milestones[1].Time).To(Equal(timing.VTimeInPicoSec(150)))

		// Draining B ends B's buffer task.
		comp.time = 160
		Expect(port.RetrieveOutgoing()).NotTo(BeNil())
		Expect(tracer.ends).To(HaveLen(2))
		Expect(tracer.ends[1].ID).To(Equal(startB.ID))
		Expect(tracer.ends[1].Time).To(Equal(timing.VTimeInPicoSec(160)))
	})

	It("parents a response's buffer task to the task it responds to", func() {
		rsp := obTestMsg{messaging.MsgMeta{ID: 9, RspTo: 7, Src: "Comp.Bottom", Dst: "Other.Top"}}

		comp.time = 200
		port.Send(rsp)

		Expect(tracer.starts).To(HaveLen(1))
		Expect(tracer.starts[0].ParentID).To(Equal(uint64(7)))
	})

	It("is a no-op when the owning component is not being traced", func() {
		untraced := &obFakeComp{name: "Untraced"}
		p2 := messaging.NewPort(untraced, 4, 4, "Untraced.Bottom")
		p2.SetConnection(&obFakeConn{})
		CollectOutgoingBufferTrace(p2)

		untraced.time = 100
		p2.Send(obTestMsg{messaging.MsgMeta{ID: 1, Src: "Untraced.Bottom", Dst: "Other.Top"}})
		Expect(p2.RetrieveOutgoing()).NotTo(BeNil())

		Expect(tracer.starts).To(BeEmpty())
		Expect(tracer.ends).To(BeEmpty())
		Expect(tracer.milestones).To(BeEmpty())
	})
})
