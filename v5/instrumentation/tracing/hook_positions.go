package tracingv5

import "github.com/sarchlab/akita/v4/v5/instrumentation/hooking"

// Hook positions for task lifecycle events
var (
	// HookPosTaskStart is triggered when a task starts execution
	HookPosTaskStart = &hooking.HookPos{Name: "TaskStart"}

	// HookPosTaskEnd is triggered when a task completes execution
	HookPosTaskEnd = &hooking.HookPos{Name: "TaskEnd"}

	// HookPosTaskTag is triggered when a task is tagged with metadata
	HookPosTaskTag = &hooking.HookPos{Name: "TaskTag"}

	// HookPosTaskStep is triggered when a task reaches a milestone/step
	HookPosTaskStep = &hooking.HookPos{Name: "TaskStep"}
)
