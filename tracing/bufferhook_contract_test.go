package tracing

import (
	"testing"

	. "github.com/onsi/gomega"

	"github.com/sarchlab/akita/v5/hooking"
	"github.com/sarchlab/akita/v5/messaging"
	"github.com/sarchlab/akita/v5/modeling"
	"github.com/sarchlab/akita/v5/timing"
)

type bufferHookCase struct {
	name         string
	newHook      func() hooking.Hook
	enter, leave *hooking.HookPos
}

func bufferHookCases() []bufferHookCase {
	return []bufferHookCase{
		{"incoming", func() hooking.Hook { return &incomingBufferHook{} },
			messaging.HookPosPortMsgRecvd, messaging.HookPosPortMsgRetrieveIncoming},
		{"outgoing", func() hooking.Hook { return &outgoingBufferHook{} },
			messaging.HookPosPortMsgSend, messaging.HookPosPortMsgRetrieveOutgoing},
	}
}

func TestBufferHooksIgnoreUnrelatedPositions(t *testing.T) {
	for _, tc := range bufferHookCases() {
		t.Run(tc.name, func(t *testing.T) {
			g := NewWithT(t)
			for _, pos := range []*hooking.HookPos{nil, HookPosTaskStart, {Name: "custom"}} {
				g.Expect(func() {
					tc.newHook().Func(hooking.HookCtx{Pos: pos, Item: "not a message"})
				}).NotTo(Panic())
			}
		})
	}
}

func TestBufferHooksRejectBrokenContracts(t *testing.T) {
	for _, tc := range bufferHookCases() {
		t.Run(tc.name, func(t *testing.T) {
			g := NewWithT(t)
			// Missing optional tracing support must not hide a malformed event.
			port := messaging.NewPort("unowned", 1, 1)
			for _, pos := range []*hooking.HookPos{tc.enter, tc.leave} {
				for _, item := range []any{nil, "not a message", &messaging.Msg{}} {
					g.Expect(func() {
						tc.newHook().Func(hooking.HookCtx{Domain: port, Pos: pos, Item: item})
					}).To(PanicWith(And(ContainSubstring(pos.Name), ContainSubstring("expected messaging.Msg"))))
				}
				g.Expect(func() {
					tc.newHook().Func(hooking.HookCtx{
						Domain: &hooking.HookableBase{}, Pos: pos, Item: messaging.Msg{},
					})
				}).To(Panic())
			}
		})
	}
}

type untracedPortOwner struct{}

func (*untracedPortOwner) NotifyRecv(messaging.Port)     {}
func (*untracedPortOwner) NotifyPortFree(messaging.Port) {}

func TestBufferHooksAllowAbsentTracing(t *testing.T) {
	for _, tc := range bufferHookCases() {
		t.Run(tc.name, func(t *testing.T) {
			g := NewWithT(t)
			for _, owner := range []messaging.PortOwner{nil, &untracedPortOwner{}, &ibFakeComp{}} {
				port := messaging.NewPort("untraced", 1, 1)
				port.SetOwner(owner)
				hook := tc.newHook()
				for _, pos := range []*hooking.HookPos{tc.enter, tc.leave} {
					g.Expect(func() {
						hook.Func(hooking.HookCtx{Domain: port, Pos: pos, Item: messaging.Msg{ID: 7}})
					}).NotTo(Panic())
				}
			}
		})
	}
}

func TestBufferHooksTraceNilPayload(t *testing.T) {
	for _, tc := range bufferHookCases() {
		t.Run(tc.name, func(t *testing.T) {
			g := NewWithT(t)
			comp := &ibFakeComp{sim: modeling.NewStandaloneSimulation(timing.NewSerialEngine()), name: "Comp"}
			tracer := &ibRecordingTracer{}
			CollectTrace(comp, tracer)
			port := messaging.NewPort("Comp.Port", 1, 1)
			port.SetOwner(comp)
			hook := tc.newHook()
			ctx := hooking.HookCtx{Domain: port, Pos: tc.enter, Item: messaging.Msg{ID: 7}}
			hook.Func(ctx)
			ctx.Pos = tc.leave
			hook.Func(ctx)
			g.Expect(tracer.starts).To(HaveLen(1))
			g.Expect(tracer.starts[0].What).To(Equal("metadata"))
			g.Expect(tracer.ends).To(HaveLen(1))
			g.Expect(tracer.ends[0].ID).To(Equal(tracer.starts[0].ID))
			t.Logf("%s: nil payload recorded as %q, task %d opened and closed",
				tc.name, tracer.starts[0].What, tracer.starts[0].ID)
		})
	}
}
