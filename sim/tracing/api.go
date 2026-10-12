package tracing

import (
	"reflect"

	"github.com/sarchlab/akita/v5/sim/hooking"
	"github.com/sarchlab/akita/v5/sim/messaging"
	"github.com/sarchlab/akita/v5/sim/naming"
	"github.com/sarchlab/akita/v5/sim/timing"
)

// NamedHookable represent something that has a name, can tell the current
// time, allocates IDs, and can be hooked. The tracing API stamps event times
// from the domain's clock, but only after confirming the domain has hooks, so
// the clock is never consulted when tracing is disabled.
type NamedHookable interface {
	naming.Named
	hooking.Hookable
	timing.TimeTeller
	InvokeHook(hooking.HookCtx)

	// NewID allocates an ID, unique within the domain's simulation.
	NewID() uint64
}

// A list of hook poses for the hooks to apply to
var (
	HookPosTaskStart = &hooking.HookPos{Name: "HookPosTaskStart"}
	HookPosTaskTag   = &hooking.HookPos{Name: "HookPosTaskTag"}
	HookPosMilestone = &hooking.HookPos{Name: "HookPosMilestone"}
	HookPosTaskEnd   = &hooking.HookPos{Name: "HookPosTaskEnd"}
)

// StartTask notifies the hooks that hook to the domain about the start of a
// task. When the task's Location is empty it is derived by [singleKindLocation]
// so that each location hosts exactly one Kind of task.
func StartTask(domain NamedHookable, t TaskStart) {
	if domain.NumHooks() == 0 {
		return
	}

	allRequiredFieldsMustBeNotEmpty(domain, t.ID, t.Kind, t.What)
	domainMustHaveName(domain)

	if t.Location == "" {
		t.Location = singleKindLocation(domain.Name(), t.Kind, t.What)
	}

	t.Time = domain.CurrentTime()

	domain.InvokeHook(hooking.HookCtx{
		Domain: domain,
		Item:   t,
		Pos:    HookPosTaskStart,
	})
}

// singleKindLocation derives a task's location so that one location only ever
// holds one Kind of task — the invariant that keeps a Daisen row sequential and
// unambiguous (see "One location, one kind" in README.md). It is consulted only
// when the caller leaves Location empty; callers that qualify the location
// themselves (the port buffer hooks append a direction) keep their own value.
//
//   - req_in and req_out share a component, so they are split by appending the
//     kind: "<component>.req_in" and "<component>.req_out".
//   - a pipeline subtask already names its stage in What as "<component>.<stage>"
//     (e.g. "L2Cache.dir_pipeline"), so that name is the location and each stage
//     is its own row.
//   - any other kind keeps the bare component name.
func singleKindLocation(componentName, kind, what string) string {
	switch kind {
	case ReqInTaskKind, ReqOutTaskKind:
		return componentName + "." + kind
	case PipelineTaskKind:
		return what
	default:
		return componentName
	}
}

func allRequiredFieldsMustBeNotEmpty(
	domain NamedHookable,
	id uint64,
	kind string,
	what string,
) {
	if id == 0 {
		panic("id must not be empty")
	}

	if domain == nil {
		panic("domain must not be nil")
	}

	if kind == "" {
		panic("kind must not be empty")
	}

	if what == "" {
		panic("what must not be empty")
	}
}

func domainMustHaveName(domain NamedHookable) {
	if domain.Name() == "" {
		panic("domain must have a name")
	}
}

// EndTask notifies the hooks about the end of a task. Zero IDs are ignored.
func EndTask(domain NamedHookable, t TaskEnd) {
	if t.ID == 0 || domain.NumHooks() == 0 {
		return
	}

	t.Time = domain.CurrentTime()

	domain.InvokeHook(hooking.HookCtx{
		Domain: domain,
		Item:   t,
		Pos:    HookPosTaskEnd,
	})
}

// AddTaskTag attaches a categorical tag to a task. A tag ID is generated when
// the caller leaves it zero. A zero TaskID is ignored.
func AddTaskTag(domain NamedHookable, tag TaskTag) {
	if tag.TaskID == 0 || domain.NumHooks() == 0 {
		return
	}

	if tag.ID == 0 {
		tag.ID = domain.NewID()
	}

	tag.Time = domain.CurrentTime()

	domain.InvokeHook(hooking.HookCtx{
		Domain: domain,
		Item:   tag,
		Pos:    HookPosTaskTag,
	})
}

// AddMilestone records the time that a blocking reason is resolved. A milestone
// ID is generated when the caller leaves it zero. The milestone's location is
// inherited from the owning task. A zero TaskID is ignored.
func AddMilestone(domain NamedHookable, m Milestone) {
	if m.TaskID == 0 || domain.NumHooks() == 0 {
		return
	}

	if m.ID == 0 {
		m.ID = domain.NewID()
	}

	m.Time = domain.CurrentTime()

	domain.InvokeHook(hooking.HookCtx{
		Domain: domain,
		Item:   m,
		Pos:    HookPosMilestone,
	})
}

// TraceReqInitiate starts the sender's req_out task and returns its ID. Store
// the ID in component State and pass it to TraceReqFinalize or EndTaskOnReset.
// Call before sending so observers see the parent before any buffer task.
// Returns zero when the domain has no hooks.
func TraceReqInitiate(domain NamedHookable, msg messaging.Msg, taskParentID uint64) uint64 {
	if domain.NumHooks() == 0 {
		return 0
	}
	id := domain.NewID()
	StartTask(domain, TaskStart{
		ID: id, MsgID: msg.ID, ParentID: taskParentID,
		Kind: ReqOutTaskKind, What: msgTypeName(msg), Detail: msg,
	})
	return id
}

// TraceReqReceive starts the receiver's req_in task and returns its ID. Store
// the ID in component State. Tracers resolve its parent from MsgID; simulation
// code never looks up the sender's task. Returns zero without hooks.
func TraceReqReceive(domain NamedHookable, msg messaging.Msg) uint64 {
	if domain.NumHooks() == 0 {
		return 0
	}
	id := domain.NewID()
	StartTask(domain, TaskStart{
		ID: id, MsgID: msg.ID, Kind: ReqInTaskKind,
		What: msgTypeName(msg), Detail: msg,
	})
	return id
}

// TraceReqComplete ends the receiver task identified by the stored task ID.
func TraceReqComplete(domain NamedHookable, taskID uint64) {
	EndTask(domain, TaskEnd{ID: taskID})
}

// TraceReqFinalize ends the sender task identified by the stored task ID.
func TraceReqFinalize(domain NamedHookable, taskID uint64) {
	EndTask(domain, TaskEnd{ID: taskID})
}

// EndTaskOnReset ends an in-flight task using its component-owned task ID.
// Zero denotes a task that was never started and is ignored. Observers ignore
// nonzero IDs whose starts they did not see (for example after a restore).
func EndTaskOnReset(domain NamedHookable, taskID uint64) {
	if taskID == 0 {
		return
	}
	EndTask(domain, TaskEnd{ID: taskID})
}

// msgTypeName returns the payload type name. Metadata-only messages are
// recorded as "metadata" so traces distinguish them from protocol traffic.
func msgTypeName(msg messaging.Msg) string {
	t := reflect.TypeOf(msg.Payload)
	if t == nil {
		return "metadata"
	}
	if name := t.Name(); name != "" {
		return name
	}
	return t.String()
}
