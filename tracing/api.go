package tracing

import (
	"reflect"

	"github.com/sarchlab/akita/v5/hooking"
	"github.com/sarchlab/akita/v5/messaging"
	"github.com/sarchlab/akita/v5/naming"
	"github.com/sarchlab/akita/v5/timing"
)

// NamedHookable represent something that has a name, can tell the current
// time, and can be hooked. The tracing API stamps event times from the
// domain's clock, but only after confirming the domain has hooks, so the
// clock is never consulted when tracing is disabled.
type NamedHookable interface {
	naming.Named
	hooking.Hookable
	timing.TimeTeller
	InvokeHook(hooking.HookCtx)
}

// A list of hook poses for the hooks to apply to
var (
	HookPosTaskStart = &hooking.HookPos{Name: "HookPosTaskStart"}
	HookPosTaskTag   = &hooking.HookPos{Name: "HookPosTaskTag"}
	HookPosMilestone = &hooking.HookPos{Name: "HookPosMilestone"}
	HookPosTaskEnd   = &hooking.HookPos{Name: "HookPosTaskEnd"}
)

// StartTask notifies the hooks that hook to the domain about the start of a
// task. When the task's Location is empty it defaults to the domain name.
func StartTask(domain NamedHookable, t TaskStart) {
	if domain.NumHooks() == 0 {
		return
	}

	allRequiredFieldsMustBeNotEmpty(t.ID, domain, t.Kind, t.What)
	domainMustHaveName(domain)

	if t.Location == "" {
		t.Location = domain.Name()
	}

	t.Time = domain.CurrentTime()

	domain.InvokeHook(hooking.HookCtx{
		Domain: domain,
		Item:   t,
		Pos:    HookPosTaskStart,
	})
}

func allRequiredFieldsMustBeNotEmpty(
	id uint64,
	domain NamedHookable,
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

// EndTask notifies the hooks about the end of a task.
func EndTask(domain NamedHookable, t TaskEnd) {
	if domain.NumHooks() == 0 {
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
// the caller leaves it zero.
func AddTaskTag(domain NamedHookable, tag TaskTag) {
	if domain.NumHooks() == 0 {
		return
	}

	if tag.ID == 0 {
		tag.ID = timing.GetIDGenerator().Generate()
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
// inherited from the owning task.
func AddMilestone(domain NamedHookable, m Milestone) {
	if domain.NumHooks() == 0 {
		return
	}

	if m.ID == 0 {
		m.ID = timing.GetIDGenerator().Generate()
	}

	m.Time = domain.CurrentTime()

	domain.InvokeHook(hooking.HookCtx{
		Domain: domain,
		Item:   m,
		Pos:    HookPosMilestone,
	})
}

// MsgIDAtReceiver returns the receiver-side task ID for the message at the
// given domain, generating one if needed. The ID is held in a tracing-local
// registry keyed by (domain, msg.Meta().ID), so the message itself is never
// mutated. When the domain has no hooks the receiver-side ID is unused, so
// this returns 0 without touching the registry — that avoids accumulating
// entries in simulations that never enable tracing.
func MsgIDAtReceiver(msg messaging.Msg, domain NamedHookable) uint64 {
	if domain.NumHooks() == 0 {
		return 0
	}

	return lookupOrCreateReceiverTaskID(msg, domain)
}

// ForgetMsgIDAtReceiver releases the registry entry created by
// [MsgIDAtReceiver] for the message identified by msgID. Use this on
// completion paths that only retain the request's message ID, not the
// message value. When the domain has no hooks no entry was ever
// inserted, so this is a no-op and avoids taking the registry mutex.
func ForgetMsgIDAtReceiver(msgID uint64, domain NamedHookable) {
	if domain.NumHooks() == 0 {
		return
	}

	forgetReceiverTaskIDByMsgID(msgID, domain)
}

// MsgIDAtIncomingBuffer returns the task ID of the buffer task that tracks a
// message's residency in the receiving port's incoming buffer. The ID lives in
// a tracing-local registry keyed by (domain, msg.Meta().ID), so the port hook
// that opens the task and the component that adds admission milestones to it
// derive the same ID without mutating the message. When the domain has no
// hooks the ID is unused, so this returns 0 without touching the registry.
func MsgIDAtIncomingBuffer(msg messaging.Msg, domain NamedHookable) uint64 {
	if domain.NumHooks() == 0 {
		return 0
	}

	return lookupOrCreateIncomingBufferTaskID(msg, domain)
}

// ForgetMsgIDAtIncomingBuffer releases the registry entry created by
// [MsgIDAtIncomingBuffer] for the message identified by msgID. The port hook
// calls this when the buffer task ends (the message is retrieved). When the
// domain has no hooks no entry was ever inserted, so this is a no-op.
func ForgetMsgIDAtIncomingBuffer(msgID uint64, domain NamedHookable) {
	if domain.NumHooks() == 0 {
		return
	}

	forgetIncomingBufferTaskIDByMsgID(msgID, domain)
}

// MsgIDAtOutgoingBuffer returns the task ID of the buffer task that tracks a
// message's residency in the sending port's outgoing buffer. The ID lives in a
// tracing-local registry keyed by (domain, msg.Meta().ID), so the port hook
// that opens the task and the one that closes it derive the same ID without
// mutating the message. When the domain has no hooks the ID is unused, so this
// returns 0 without touching the registry.
func MsgIDAtOutgoingBuffer(msg messaging.Msg, domain NamedHookable) uint64 {
	if domain.NumHooks() == 0 {
		return 0
	}

	return lookupOrCreateOutgoingBufferTaskID(msg, domain)
}

// ForgetMsgIDAtOutgoingBuffer releases the registry entry created by
// [MsgIDAtOutgoingBuffer] for the message identified by msgID. The port hook
// calls this when the buffer task ends (the message is drained by the
// connection). When the domain has no hooks no entry was ever inserted, so this
// is a no-op.
func ForgetMsgIDAtOutgoingBuffer(msgID uint64, domain NamedHookable) {
	if domain.NumHooks() == 0 {
		return
	}

	forgetOutgoingBufferTaskIDByMsgID(msgID, domain)
}

// TraceReqInitiate marks a task starting at the sender of a message. The
// task ID is the message's own ID, which is fixed at message construction.
// The task kind is "req_out".
func TraceReqInitiate(
	domain NamedHookable,
	msg messaging.Msg,
	taskParentID uint64,
) {
	if domain.NumHooks() == 0 {
		return
	}

	StartTask(domain, TaskStart{
		ID:       msg.Meta().ID,
		ParentID: taskParentID,
		Kind:     "req_out",
		What:     msgTypeName(msg),
		Detail:   msg,
	})
}

// TraceReqReceive marks a task starting at the receiver of a message. The
// parent is the sender's req_out task, identified by the message's ID. The
// task kind is "req_in".
func TraceReqReceive(
	domain NamedHookable,
	msg messaging.Msg,
) {
	if domain.NumHooks() == 0 {
		return
	}

	StartTask(domain, TaskStart{
		ID:       MsgIDAtReceiver(msg, domain),
		ParentID: msg.Meta().ID,
		Kind:     "req_in",
		What:     msgTypeName(msg),
		Detail:   msg,
	})
}

// TraceReqComplete terminates the receiver-side handling task for a message
// and releases the registry entry held for it.
func TraceReqComplete(
	domain NamedHookable,
	msg messaging.Msg,
) {
	if domain.NumHooks() == 0 {
		return
	}

	EndTask(domain, TaskEnd{ID: MsgIDAtReceiver(msg, domain)})
	forgetReceiverTaskID(msg, domain)
}

// TraceReqFinalize terminates the sender-side task for a message. The sender
// calls this when the response arrives.
func TraceReqFinalize(
	domain NamedHookable,
	msg messaging.Msg,
) {
	if domain.NumHooks() == 0 {
		return
	}

	EndTask(domain, TaskEnd{ID: msg.Meta().ID})
}

// EndReqInOnReset ends the receiver-side (req_in) task for an in-flight request
// whose message ID is reqMsgID and releases the registry entry held for it. A
// reset that drops in-flight work — whose responses will be discarded — calls
// this to tear the task down now, mirroring [TraceReqComplete], rather than
// leaving it started-never-ended and its registry entry leaked. It resolves the
// task ID by message ID (the receiver registry), since reset paths retain the
// request's ID, not its message value. It is a no-op when the domain has no
// hooks or no task is registered for the ID (the request never opened a req_in,
// or already completed).
func EndReqInOnReset(domain NamedHookable, reqMsgID uint64) {
	if domain.NumHooks() == 0 {
		return
	}

	id, ok := receiverTaskIDByMsgID(reqMsgID, domain)
	if !ok {
		return
	}

	EndTask(domain, TaskEnd{ID: id})
	forgetReceiverTaskIDByMsgID(reqMsgID, domain)
}

// EndTaskOnReset ends an in-flight task by its task ID on a reset path that
// drops the work the task tracks. Use it for every open task that is keyed on
// its own ID rather than the receiver registry: a forwarded request's
// sender-side req_out (task ID == the downstream message's own ID, like
// [TraceReqFinalize]), a DRAM sub-transaction, a cache_transaction, or a
// pipeline subtask. The req_in task is the only kind that also needs its
// registry entry released — use [EndReqInOnReset] for that. It is a no-op when
// the domain has no hooks or the task was never started, so a caller may end
// every task a transaction could hold without tracking which were opened.
func EndTaskOnReset(domain NamedHookable, taskID uint64) {
	if domain.NumHooks() == 0 {
		return
	}

	EndTask(domain, TaskEnd{ID: taskID})
}

// msgTypeName returns the Go type name of the message's underlying type,
// transparently unwrapping pointers so both value- and pointer-typed
// implementations of [messaging.Msg] yield a non-empty name.
func msgTypeName(msg messaging.Msg) string {
	t := reflect.TypeOf(msg)
	if t.Kind() == reflect.Pointer {
		t = t.Elem()
	}

	return t.Name()
}
