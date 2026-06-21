package tracing

import (
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/sarchlab/akita/v5/messaging"
	"github.com/sarchlab/akita/v5/timing"
)

// resetTestMsg is a minimal message used to drive the reset helpers through the
// real receiver-task registry and a recording tracer.
type resetTestMsg struct {
	messaging.MsgMeta
}

var _ = Describe("Reset task-cleanup helpers", func() {
	var (
		comp   *ibFakeComp
		tracer *ibRecordingTracer
	)

	BeforeEach(func() {
		comp = &ibFakeComp{name: "ResetComp"}
		tracer = &ibRecordingTracer{}
		CollectTrace(comp, tracer)
	})

	Describe("EndReqInOnReset", func() {
		It("ends the in-flight req_in task and forgets its registry entry", func() {
			req := resetTestMsg{messaging.MsgMeta{ID: 7}}
			comp.time = 100
			TraceReqReceive(comp, req)

			// The task started and its registry entry exists.
			taskID, ok := receiverTaskIDByMsgID(7, comp)
			Expect(ok).To(BeTrue())
			Expect(tracer.starts).To(HaveLen(1))
			Expect(tracer.starts[0].ID).To(Equal(taskID))
			Expect(tracer.starts[0].Kind).To(Equal("req_in"))

			comp.time = 150
			EndReqInOnReset(comp, 7)

			// The task ended at reset time and the registry entry is gone.
			Expect(tracer.ends).To(HaveLen(1))
			Expect(tracer.ends[0].ID).To(Equal(taskID))
			Expect(tracer.ends[0].Time).To(Equal(timing.VTimeInPicoSec(150)))
			_, ok = receiverTaskIDByMsgID(7, comp)
			Expect(ok).To(BeFalse())
		})

		It("is a no-op for a message that opened no req_in", func() {
			comp.time = 150
			EndReqInOnReset(comp, 999)

			Expect(tracer.ends).To(BeEmpty())
			// No phantom registry entry is created by the lookup.
			_, ok := receiverTaskIDByMsgID(999, comp)
			Expect(ok).To(BeFalse())
		})

		It("matches the success-path completion for the same request", func() {
			// TraceReqComplete and EndReqInOnReset must end the same task ID and
			// both clear the registry, so a reset is indistinguishable in the
			// trace from a normal completion except for the end time.
			req := resetTestMsg{messaging.MsgMeta{ID: 11}}
			TraceReqReceive(comp, req)
			taskID, _ := receiverTaskIDByMsgID(11, comp)

			EndReqInOnReset(comp, 11)

			Expect(tracer.ends).To(HaveLen(1))
			Expect(tracer.ends[0].ID).To(Equal(taskID))
			_, ok := receiverTaskIDByMsgID(11, comp)
			Expect(ok).To(BeFalse())
		})
	})

	Describe("EndTaskOnReset", func() {
		It("ends the in-flight req_out task by the message's own ID", func() {
			reqToBottom := resetTestMsg{messaging.MsgMeta{ID: 42}}
			comp.time = 100
			TraceReqInitiate(comp, reqToBottom, 0)

			Expect(tracer.starts).To(HaveLen(1))
			Expect(tracer.starts[0].ID).To(Equal(uint64(42)))
			Expect(tracer.starts[0].Kind).To(Equal("req_out"))

			comp.time = 160
			EndTaskOnReset(comp, 42)

			Expect(tracer.ends).To(HaveLen(1))
			Expect(tracer.ends[0].ID).To(Equal(uint64(42)))
			Expect(tracer.ends[0].Time).To(Equal(timing.VTimeInPicoSec(160)))
		})

		It("ends a subtask (e.g. sub-trans or pipeline) by its own task ID", func() {
			comp.time = 100
			StartTask(comp, TaskStart{
				ID: 77, ParentID: 5, Kind: "sub-trans", What: "subtrans",
			})

			comp.time = 170
			EndTaskOnReset(comp, 77)

			Expect(tracer.ends).To(HaveLen(1))
			Expect(tracer.ends[0].ID).To(Equal(uint64(77)))
		})

		It("is a no-op when the domain has no hooks", func() {
			untraced := &ibFakeComp{name: "Untraced"}
			EndTaskOnReset(untraced, 12345)
			Expect(tracer.ends).To(BeEmpty())
		})

		It("emits the end unconditionally; the consumer ignores unknown IDs", func() {
			// Unlike EndReqInOnReset, EndTaskOnReset cannot tell whether the
			// task was started — req_out/subtask IDs have no registry entry — so
			// it always emits. The DBTracer drops ends for IDs it never saw
			// start, which is what makes blanket end-on-reset safe.
			EndTaskOnReset(comp, 12345)
			Expect(tracer.ends).To(HaveLen(1))
			Expect(tracer.ends[0].ID).To(Equal(uint64(12345)))
		})
	})
})
