package tracing

import "gitlab.com/akita/akita/v3/sim"

// CollectTrace let the tracer to collect trace from a domain
func CollectTrace(domain NamedHookable, tracer Tracer) {
	h := traceHook{t: tracer}
	domain.AcceptHook(&h)
}

// A traceHook is a hook that traces tasks
type traceHook struct {
	t Tracer
}

// Func calls the tracer interfaces when the hook is triggered
func (h *traceHook) Func(ctx sim.HookCtx) {
	switch ctx.Pos {
	case HookPosTaskStart:
		h.t.StartTask(ctx.Item.(Task))
	case HookPosTaskStep:
		h.t.StepTask(ctx.Item.(Task))
	case HookPosTaskEnd:
		h.t.EndTask(ctx.Item.(Task))
	}
}
