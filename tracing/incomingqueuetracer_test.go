package tracing

import (
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/sarchlab/akita/v5/hooking"
	"github.com/sarchlab/akita/v5/messaging"
	"github.com/sarchlab/akita/v5/timing"
)

// iqFakeComp is a minimal component that satisfies both messaging.Component
// (so it can own a real port) and NamedHookable (so the tracing API can emit
// tasks on it). InvokeHook is provided by the embedded HookableBase, which is
// how CollectTrace forwards events to a tracer.
type iqFakeComp struct {
	hooking.HookableBase
	name string
	time timing.VTimeInPicoSec
}

func (c *iqFakeComp) Name() string                       { return c.name }
func (c *iqFakeComp) CurrentTime() timing.VTimeInPicoSec { return c.time }

func (c *iqFakeComp) DeclarePort(string, ...*messaging.Role) {}
func (c *iqFakeComp) AssignPort(string, messaging.Port)      {}
func (c *iqFakeComp) GetPortByName(string) messaging.Port    { return nil }
func (c *iqFakeComp) Ports() []messaging.Port                { return nil }
func (c *iqFakeComp) NotifyRecv(messaging.Port)              {}
func (c *iqFakeComp) NotifyPortFree(messaging.Port)          {}

// iqRecordingTracer captures the task events the hook produces.
type iqRecordingTracer struct {
	NopTracer
	starts []TaskStart
	ends   []TaskEnd
}

func (t *iqRecordingTracer) StartTask(ts TaskStart) { t.starts = append(t.starts, ts) }
func (t *iqRecordingTracer) EndTask(te TaskEnd)     { t.ends = append(t.ends, te) }

type iqTestMsg struct {
	messaging.MsgMeta
}

var _ = Describe("Incoming queue tracer", func() {
	var (
		comp   *iqFakeComp
		tracer *iqRecordingTracer
		port   messaging.Port
	)

	BeforeEach(func() {
		comp = &iqFakeComp{name: "Comp"}
		tracer = &iqRecordingTracer{}
		CollectTrace(comp, tracer)

		port = messaging.NewPort(comp, 4, 4, "Comp.Top")
		CollectIncomingQueueTrace(port)
	})

	It("ends a message's queueing when it reaches the head: zero for a "+
		"message delivered to an empty buffer, and spanning the wait for one "+
		"delivered behind another", func() {
		// A lands in an empty buffer => immediately at the head.
		a := iqTestMsg{messaging.MsgMeta{ID: 7, Dst: "Comp.Top"}}
		comp.time = 100
		port.Deliver(a)

		// B lands behind A => waits until A is retrieved.
		b := iqTestMsg{messaging.MsgMeta{ID: 8, Dst: "Comp.Top"}}
		comp.time = 110
		port.Deliver(b)

		Expect(tracer.starts).To(HaveLen(2))
		startA := tracer.starts[0]
		Expect(startA.Kind).To(Equal(IncomingQueueTaskKind))
		Expect(startA.ParentID).To(Equal(uint64(7)))
		Expect(startA.What).To(Equal("iqTestMsg"))
		Expect(startA.Location).To(Equal("Comp.Top"))
		Expect(startA.Time).To(Equal(timing.VTimeInPicoSec(100)))
		startB := tracer.starts[1]
		Expect(startB.ParentID).To(Equal(uint64(8)))
		Expect(startB.Time).To(Equal(timing.VTimeInPicoSec(110)))

		// A's task already closed at delivery (it was at the head).
		Expect(tracer.ends).To(HaveLen(1))
		Expect(tracer.ends[0].ID).To(Equal(startA.ID))
		Expect(tracer.ends[0].Time).To(Equal(timing.VTimeInPicoSec(100)))

		// Retrieving A exposes B at the head, closing B's queueing.
		comp.time = 150
		Expect(port.RetrieveIncoming()).NotTo(BeNil())

		Expect(tracer.ends).To(HaveLen(2))
		Expect(tracer.ends[1].ID).To(Equal(startB.ID))
		Expect(tracer.ends[1].Time).To(Equal(timing.VTimeInPicoSec(150)))
	})

	It("parents a response's queueing task to the task it responds to", func() {
		rsp := iqTestMsg{messaging.MsgMeta{ID: 9, RspTo: 7, Dst: "Comp.Top"}}

		comp.time = 200
		port.Deliver(rsp)

		Expect(tracer.starts).To(HaveLen(1))
		Expect(tracer.starts[0].ParentID).To(Equal(uint64(7)))
	})

	It("is a no-op when the owning component is not being traced", func() {
		untraced := &iqFakeComp{name: "Untraced"}
		p2 := messaging.NewPort(untraced, 4, 4, "Untraced.Top")
		CollectIncomingQueueTrace(p2)

		untraced.time = 100
		p2.Deliver(iqTestMsg{messaging.MsgMeta{ID: 1, Dst: "Untraced.Top"}})
		Expect(p2.RetrieveIncoming()).NotTo(BeNil())

		Expect(tracer.starts).To(BeEmpty())
		Expect(tracer.ends).To(BeEmpty())
	})
})
