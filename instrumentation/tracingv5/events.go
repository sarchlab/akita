package tracingv5

// TaskStart represents the event when a task starts execution.
// It is used as the item in HookPosTaskStart hooks.
type TaskStart struct {
	ID       string // Unique identifier for the task
	What     string // Description of what the task does
	Where    string // Component/location where the task is executing
	ParentID string // ID of the parent task (empty if no parent)
	Detail   any    // Optional task-specific details
}

// TaskEnd represents the event when a task completes execution.
// It is used as the item in HookPosTaskEnd hooks.
// Only ID is needed. Other fields are provided in TaskStart.
type TaskEnd struct {
	ID string // Unique identifier for the task (matches TaskStart.ID)
}

// TaskTag represents metadata tagging for a task.
// It is used as the item in HookPosTaskTag hooks.
type TaskTag struct {
	TaskID string // ID of the task being tagged
	Tag    string // The tag value
}

// StepBlockingReason categorizes why a task step was blocked/delayed.
type StepBlockingReason string

const (
	// StepBlockedByHardware indicates the step was waiting for hardware resources
	StepBlockedByHardware StepBlockingReason = "hardware_resource"

	// StepBlockedByNetworkTransfer indicates the step was waiting for network transfer
	StepBlockedByNetworkTransfer StepBlockingReason = "network_transfer"

	// StepBlockedByNetworkBusy indicates the step was waiting because network was busy
	StepBlockedByNetworkBusy StepBlockingReason = "network_busy"

	// StepBlockedByQueue indicates the step was waiting in a queue
	StepBlockedByQueue StepBlockingReason = "queue"

	// StepBlockedByData indicates the step was waiting for data
	StepBlockedByData StepBlockingReason = "data"

	// StepBlockedByDependency indicates the step was waiting for dependencies
	StepBlockedByDependency StepBlockingReason = "dependency"

	// StepBlockedByTranslation indicates the step was waiting for address translation
	StepBlockedByTranslation StepBlockingReason = "translation"

	// StepBlockedBySubTask indicates the step was waiting for a subtask
	StepBlockedBySubTask StepBlockingReason = "subtask"

	// StepBlockedByOther indicates the step was blocked for another reason
	StepBlockedByOther StepBlockingReason = "other"
)

// TaskStep represents a milestone or intermediate step within a task.
// It is used as the item in HookPosTaskStep hooks.
//
// A step marks a point where a task's blocking status is resolved. The
// BlockingReason indicates the category of what blocked the task (e.g.,
// hardware resource, network transfer, queue), while BlockingDetail
// provides specific information about that blocker (e.g., "ALU Unit 3",
// "Memory Controller Port 1").
type TaskStep struct {
	ID              string             // Unique identifier for the step
	TaskID          string             // ID of the task this step belongs to
	BlockingReason  StepBlockingReason // Category of what blocked the task
	BlockingDetail  string             // Specific description of the blocker
	Detail          any                // Optional step-specific details
}
