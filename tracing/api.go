package tracing

import (
	"reflect"

	"github.com/sarchlab/akita/v5/hooking"
	"github.com/sarchlab/akita/v5/messaging"
	"github.com/sarchlab/akita/v5/naming"
	"github.com/sarchlab/akita/v5/timing"
)

// NamedHookable represent something both have a name and can be hooked
type NamedHookable interface {
	naming.Named
	hooking.Hookable
	InvokeHook(hooking.HookCtx)
}

// A list of hook poses for the hooks to apply to
var (
	HookPosTaskStart = &hooking.HookPos{Name: "HookPosTaskStart"}
	HookPosTaskStep  = &hooking.HookPos{Name: "HookPosTaskStep"}
	HookPosMilestone = &hooking.HookPos{Name: "HookPosMilestone"}
	HookPosTaskEnd   = &hooking.HookPos{Name: "HookPosTaskEnd"}
)

// StartTask notifies the hooks that hook to the domain about the start of a
// task.
func StartTask(
	id uint64,
	parentID uint64,
	domain NamedHookable,
	kind string,
	what string,
	detail interface{},
) {
	StartTaskWithSpecificLocation(
		id,
		parentID,
		domain,
		kind,
		what,
		domain.Name(),
		detail,
	)
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

// StartTaskWithSpecificLocation notifies the hooks that hook to the domain
// about the start of a task, and is able to customize `where` field of a task,
// especially for network tracing.
func StartTaskWithSpecificLocation(
	id uint64,
	parentID uint64,
	domain NamedHookable,
	kind string,
	what string,
	location string,
	detail interface{},
) {
	if domain.NumHooks() == 0 {
		return
	}

	allRequiredFieldsMustBeNotEmpty(id, domain, kind, what)
	domainMustHaveName(domain)

	task := Task{
		ID:       id,
		ParentID: parentID,
		Kind:     kind,
		What:     what,
		Location: location,
		Detail:   detail,
	}
	ctx := hooking.HookCtx{
		Domain: domain,
		Item:   task,
		Pos:    HookPosTaskStart,
	}
	domain.InvokeHook(ctx)
}

// AddTaskStep marks that a milestone has been reached when processing a task.
func AddTaskStep(
	id uint64,
	domain NamedHookable,
	what string,
) {
	if domain.NumHooks() == 0 {
		return
	}

	step := TaskStep{
		What: what,
	}
	task := Task{
		ID:    id,
		Steps: []TaskStep{step},
	}
	ctx := hooking.HookCtx{
		Domain: domain,
		Item:   task,
		Pos:    HookPosTaskStep,
	}
	domain.InvokeHook(ctx)
}

// AddMilestone records the time that that a blocking reason is resolved.
func AddMilestone(
	taskID uint64,
	kind MilestoneKind,
	what string,
	location string,
	domain NamedHookable,
) {
	if domain.NumHooks() == 0 {
		return
	}

	milestone := Milestone{
		ID:       timing.GetIDGenerator().Generate(),
		TaskID:   taskID,
		Kind:     kind,
		What:     what,
		Location: location,
	}

	ctx := hooking.HookCtx{
		Domain: domain,
		Item:   milestone,
		Pos:    HookPosMilestone,
	}
	domain.InvokeHook(ctx)
}

// EndTask notifies the hooks about the end of a task.
func EndTask(
	id uint64,
	domain NamedHookable,
) {
	if domain.NumHooks() == 0 {
		return
	}

	task := Task{
		ID: id,
	}
	ctx := hooking.HookCtx{
		Domain: domain,
		Item:   task,
		Pos:    HookPosTaskEnd,
	}
	domain.InvokeHook(ctx)
}

// MsgIDAtReceiver returns the receiver-side task ID for the message at the
// given domain, generating one if needed. The ID is held in a tracing-local
// registry keyed by (domain, msg.Meta().ID), so the message itself is never
// mutated.
func MsgIDAtReceiver(msg messaging.Msg, domain NamedHookable) uint64 {
	return lookupOrCreateReceiverTaskID(msg, domain)
}

// TraceReqInitiate marks a task starting at the sender of a message. The
// task ID is the message's own ID, which is fixed at message construction.
// The task kind is "req_out".
func TraceReqInitiate(
	msg messaging.Msg,
	domain NamedHookable,
	taskParentID uint64,
) {
	if domain.NumHooks() == 0 {
		return
	}

	StartTask(
		msg.Meta().ID,
		taskParentID,
		domain,
		"req_out",
		reflect.TypeOf(msg).Name(),
		msg,
	)
}

// TraceReqReceive marks a task starting at the receiver of a message. The
// parent is the sender's req_out task, identified by the message's ID. The
// task kind is "req_in".
func TraceReqReceive(
	msg messaging.Msg,
	domain NamedHookable,
) {
	if domain.NumHooks() == 0 {
		return
	}

	StartTask(
		MsgIDAtReceiver(msg, domain),
		msg.Meta().ID,
		domain,
		"req_in",
		reflect.TypeOf(msg).Name(),
		msg,
	)
}

// TraceReqComplete terminates the receiver-side handling task for a message
// and releases the registry entry held for it.
func TraceReqComplete(
	msg messaging.Msg,
	domain NamedHookable,
) {
	EndTask(MsgIDAtReceiver(msg, domain), domain)
	forgetReceiverTaskID(msg, domain)
}

// TraceReqFinalize terminates the sender-side task for a message. The sender
// calls this when the response arrives.
func TraceReqFinalize(
	msg messaging.Msg,
	domain NamedHookable,
) {
	EndTask(msg.Meta().ID, domain)
}
