package tracing

import (
	"fmt"
	"reflect"

	"github.com/sarchlab/akita/v3/sim"
)

// NamedHookable represent something both have a name and can be hooked
type NamedHookable interface {
	sim.Named
	sim.Hookable
	InvokeHook(sim.HookCtx)
}

// A list of hook poses for the hooks to apply to
var (
	HookPosTaskStart = &sim.HookPos{Name: "HookPosTaskStart"}
	HookPosTaskStep  = &sim.HookPos{Name: "HookPosTaskStep"}
	HookPosTaskEnd   = &sim.HookPos{Name: "HookPosTaskEnd"}
	HookPosTaskDelay  = &sim.HookPos{Name: "HookPosTaskDelay"}
	HookPosTaskProgress  = &sim.HookPos{Name: "HookPosTaskProgress"}
	HookPosTaskDependency = &sim.HookPos{Name: "HookPosTaskDependency"}
)

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

func DelayTask(
	msg sim.Msg,
	domain NamedHookable,
	portInfo string,
	now sim.VTimeInSec,
	processEvent string,
	delayType string,
	sourceFile string,
) {
	var taskID string
	var addressInfo string 

	if msg != nil {
		taskID = msg.Meta().ID 
		// if readReq, ok := msg.(*mem.ReadReq); ok {
		// 	msgAddress := readReq.Address
		// 	addressInfo = strconv.FormatUint(msgAddress, 10) 
		// }
	} else {
		taskID = ""
	}
	
	delayEvent := DelayEvent{
		EventID: processEvent,
		TaskID:  taskID,
		Type:    delayType, 
		What:    "addressInfo: "+ addressInfo +" portInfo: "+ portInfo, 
		Source:  domain.Name(), 
		Time:    now,
	}
	ctx := sim.HookCtx{
		Domain: domain,
		Item:   delayEvent,
		Pos:    HookPosTaskDelay,
	}
	domain.InvokeHook(ctx)
}


func ProgressTask(
	progressID string,
	taskID string,
	domain NamedHookable,
	now sim.VTimeInSec,
	reason string,
) {
	// var taskID string

	progressEvent := ProgressEvent{
		ProgressID: progressID,
		TaskID:  taskID,
		Source:  domain.Name(), 
		Time:    now,
		Reason:  reason,
	}
	ctx := sim.HookCtx{
		Domain: domain,
		Item:   progressEvent,
		Pos:    HookPosTaskProgress,
	}
	domain.InvokeHook(ctx)
}

func DependencyTask(
	progressID string, 
	domain NamedHookable,
	dependentIDs []string,
) {
	dependencyEvent := DependencyEvent{
		ProgressID: progressID,
		DependentID:  dependentIDs,
	}
	ctx := sim.HookCtx{
		Domain: domain,
		Item:   dependencyEvent,
		Pos:    HookPosTaskDependency,
	}
	domain.InvokeHook(ctx)
}

// MsgIDAtReceiver generates a standard ID for the message task at the
// message receiver.
func MsgIDAtReceiver(msg sim.Msg, domain NamedHookable) string {
	fmt.Println("MsgIDAtReceiver Message ID:", msg.Meta().ID)
	fmt.Println("MsgIDAtReceiver Message src name:", msg.Meta().Src.Name())
	fmt.Println("MsgIDAtReceiver Message domain name:", domain.Name())
	return fmt.Sprintf("%s@%s", msg.Meta().ID, domain.Name())
}

// TraceReqInitiate generatse a new task. The new task has Type="req_out",
// What=[the type name of the message]. This function is to be called by the
// sender of the message.
func TraceReqInitiate(
	msg sim.Msg,
	domain NamedHookable,
	taskParentID string,
)string {
	fmt.Println("TraceReqInitiate StartTask Message ID:", msg.Meta().ID)
	taskID := msg.Meta().ID+"_req_out";
	StartTask(
		taskID,
		taskParentID,
		domain,
		"req_out",
		reflect.TypeOf(msg).String(),
		msg,
	)
	return taskID
}

// TraceReqReceive generates a new task for the message handling. The kind of
// the task is always "req_in".
func TraceReqReceive(
	msg sim.Msg,
	domain NamedHookable,
) {
	fmt.Println("TraceReqReceive StartTask Message ID:", msg.Meta().ID)
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
	fmt.Println("TraceReqComplete EndTask Message ID:")
	EndTask(MsgIDAtReceiver(msg, domain), domain)
}

// TraceReqFinalize terminates the message task. This function should be called
// when the sender receives the response.
func TraceReqFinalize(
	msg sim.Msg,
	domain NamedHookable,
) string {
	fmt.Println("TraceReqFinalize EndTask Message ID:")
	taskID := msg.Meta().ID+"_req_out";
	EndTask(taskID, domain)
	return taskID
}


func TraceDelay(
	msg sim.Msg,
	domain NamedHookable,
	portInfo string,
	now sim.VTimeInSec,
	processEvent string,
	delayType string,
	sourceFile string,
) {
	DelayTask(msg, domain, portInfo, now, processEvent, delayType, sourceFile)
}

func TraceProgress(
	progressID string,
	receiverTaskID string,
	// msg sim.Msg,
	domain NamedHookable,
	now sim.VTimeInSec,
	sourceFile string,
	reason string,
) {
	ProgressTask(progressID, receiverTaskID, domain, now, reason)
}


func TraceDependency(
	progressID string,
	domain NamedHookable,
	dependentIDs []string,
) {
	DependencyTask(progressID, domain, dependentIDs)
}