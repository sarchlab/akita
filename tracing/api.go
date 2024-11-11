package tracing

import (
	"fmt"
	"reflect"
	"sync/atomic"
	"sync"
	"github.com/sarchlab/akita/v4/sim"
	"strconv"
)

// NamedHookable represent something both have a name and can be hooked
type NamedHookable interface {
	sim.Named
	sim.Hookable
	InvokeHook(sim.HookCtx)
	CurrentTime() sim.VTimeInSec
}

type Milestone struct {
    ID                string
    TaskID            string
    BlockingCategory  string
    BlockingReason    string
    BlockingLocation  string
	Time              float64
}

// A list of hook poses for the hooks to apply to
var (
	HookPosTaskStart = &sim.HookPos{Name: "HookPosTaskStart"}
	HookPosTaskStep  = &sim.HookPos{Name: "HookPosTaskStep"}
	HookPosTaskEnd   = &sim.HookPos{Name: "HookPosTaskEnd"}
)

var (
    milestones       []Milestone
    milestonesMutex  sync.Mutex
    milestoneIDCounter uint64
)


func AddMilestone(
    taskID           string,
    blockingCategory string,
    blockingReason   string,
    blockingLocation string,
    domain           NamedHookable,
) {
    currentTime := float64(domain.CurrentTime())
    
    milestone := Milestone{
        ID:               strconv.FormatUint(generateMilestoneID(), 10),
        TaskID:           taskID,
        BlockingCategory: blockingCategory,
        BlockingReason:   blockingReason,
        BlockingLocation: blockingLocation,
		Time:             currentTime,
    }
	fmt.Printf("Milestone added: ID=%s, TaskID=%s, Category=%s, Reason=%s, Location=%s, Time=%f\n",
	milestone.ID, milestone.TaskID, milestone.BlockingCategory, milestone.BlockingReason, milestone.BlockingLocation, milestone.Time)
    milestonesMutex.Lock()
	milestones = append(milestones, milestone)
	milestonesMutex.Unlock()

	ctx := sim.HookCtx{
        Domain: domain,
        Item:   milestone,
        Pos:    HookPosMilestone,
    }
    domain.InvokeHook(ctx)
}

var HookPosMilestone = &sim.HookPos{Name: "HookPosMilestone"}
func generateMilestoneID() uint64 {
    return atomic.AddUint64(&milestoneIDCounter, 1)
}

func GetAllMilestones() []Milestone {
    return milestones
}

// StartTask notifies the hooks that hook to the domain about the start of a
// task.
func StartTask(
	id string,
	parentID string,
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
	id string,
	domain NamedHookable,
	kind string,
	what string,
) {
	if id == "" {
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
	id string,
	parentID string,
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
		Where:    location,
		Detail:   detail,
	}
	ctx := sim.HookCtx{
		Domain: domain,
		Item:   task,
		Pos:    HookPosTaskStart,
	}
	domain.InvokeHook(ctx)
}

// AddTaskStep marks that a milestone has been reached when processing a task.
func AddTaskStep(
	id string,
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
	ctx := sim.HookCtx{
		Domain: domain,
		Item:   task,
		Pos:    HookPosTaskStep,
	}
	domain.InvokeHook(ctx)
}

// EndTask notifies the hooks about the end of a task.
func EndTask(
	id string,
	domain NamedHookable,
) {
	if domain.NumHooks() == 0 {
		return
	}

	task := Task{
		ID: id,
	}
	ctx := sim.HookCtx{
		Domain: domain,
		Item:   task,
		Pos:    HookPosTaskEnd,
	}
	domain.InvokeHook(ctx)
}

// MsgIDAtReceiver generates a standard ID for the message task at the
// message receiver.
func MsgIDAtReceiver(msg sim.Msg, domain NamedHookable) string {
	return fmt.Sprintf("%s@%s", msg.Meta().ID, domain.Name())
}

// TraceReqInitiate generatse a new task. The new task has Type="req_out",
// What=[the type name of the message]. This function is to be called by the
// sender of the message.
func TraceReqInitiate(
	msg sim.Msg,
	domain NamedHookable,
	taskParentID string,
) {
	StartTask(
		msg.Meta().ID+"_req_out",
		taskParentID,
		domain,
		"req_out",
		reflect.TypeOf(msg).String(),
		msg,
	)
}

// TraceReqReceive generates a new task for the message handling. The kind of
// the task is always "req_in".
func TraceReqReceive(
	msg sim.Msg,
	domain NamedHookable,
) {
	StartTask(
		MsgIDAtReceiver(msg, domain),
		msg.Meta().ID+"_req_out",
		domain,
		"req_in",
		reflect.TypeOf(msg).String(),
		msg,
	)
}

// TraceReqComplete terminates the message handling task.
func TraceReqComplete(
	msg sim.Msg,
	domain NamedHookable,
) {
	EndTask(MsgIDAtReceiver(msg, domain), domain)
}

// TraceReqFinalize terminates the message task. This function should be called
// when the sender receives the response.
func TraceReqFinalize(
	msg sim.Msg,
	domain NamedHookable,
) {
	EndTask(msg.Meta().ID+"_req_out", domain)
}
