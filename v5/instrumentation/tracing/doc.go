// Package tracingv5 provides hook-based task tracing infrastructure.
//
// This package defines hook positions and data structures for tracing task
// execution in simulations. Unlike the legacy tracing package which provides
// tracer APIs, tracingv5 relies entirely on the hooking infrastructure for
// extensibility.
//
// # Hook Positions
//
// The package defines hook positions for task-level and request-level tracing:
//
// Task-level hooks:
//   - HookPosTaskStart: Triggered when a task begins execution
//   - HookPosTaskEnd: Triggered when a task completes
//   - HookPosTaskTag: Triggered when metadata is added to a task
//   - HookPosTaskStep: Triggered when a task reaches a milestone
//
// Request-level hooks:
//   - HookPosReqInitiate: Triggered when a component sends a request
//   - HookPosReqReceive: Triggered when a component receives a request
//   - HookPosReqComplete: Triggered when request handling completes
//   - HookPosReqFinalize: Triggered when the sender receives the response
//
// # Event Structures
//
// Task events:
//   - TaskStart: Contains task ID, description, location, and parent information
//   - TaskEnd: Contains task ID only (other info in TaskStart)
//   - TaskTag: Contains task ID and tag value
//   - TaskStep: Contains step ID, task ID, blocking reason, and details
//
// Request events:
//   - ReqInitiate: Contains message, location, and parent task ID
//   - ReqReceive: Contains message and location
//   - ReqComplete: Contains message only
//   - ReqFinalize: Contains message only
//
// # Request Tracing Pattern
//
// Request tracing tracks message lifecycles across components. Each message
// creates two tasks:
//
//  1. Send task (sender's perspective): ReqInitiate → ReqFinalize
//  2. Receive task (receiver's perspective): ReqReceive → ReqComplete
//
// Task IDs are generated using helper functions:
//   - SendReqTaskID(msg): Returns msg.Meta().ID + "_send"
//   - ReceiveReqTaskID(msg): Returns msg.Meta().ID + "_recv"
//
// Example request tracing flow:
//
//	// Sender initiates request
//	component.InvokeHook(hooking.HookCtx{
//	    Domain: component,
//	    Pos:    tracingv5.HookPosReqInitiate,
//	    Item: tracingv5.ReqInitiate{
//	        Where:        component.Name(),
//	        Msg:          request,
//	        ParentTaskID: "parent-task-123",
//	    },
//	})
//
//	// Receiver handles request
//	component.InvokeHook(hooking.HookCtx{
//	    Domain: component,
//	    Pos:    tracingv5.HookPosReqReceive,
//	    Item: tracingv5.ReqReceive{
//	        Where: component.Name(),
//	        Msg:   request,
//	    },
//	})
//
//	// Receiver completes handling
//	component.InvokeHook(hooking.HookCtx{
//	    Domain: component,
//	    Pos:    tracingv5.HookPosReqComplete,
//	    Item:   tracingv5.ReqComplete{Msg: request},
//	})
//
//	// Sender receives response
//	component.InvokeHook(hooking.HookCtx{
//	    Domain: component,
//	    Pos:    tracingv5.HookPosReqFinalize,
//	    Item:   tracingv5.ReqFinalize{Msg: request},
//	})
//
// # Usage Example
//
// Components fire task events using the hooking API:
//
//	component.InvokeHook(hooking.HookCtx{
//	    Domain: component,
//	    Pos:    tracingv5.HookPosTaskStart,
//	    Item: tracingv5.TaskStart{
//	        ID:    "task-123",
//	        What:  "Memory read",
//	        Where: component.Name(),
//	    },
//	})
//
// Tracers register hooks to observe these events by implementing the Hook interface:
//
//	type MyHook struct {}
//
//	func (h *MyHook) Func(ctx hooking.HookCtx) {
//	    if ctx.Pos == tracingv5.HookPosTaskStart {
//	        if event, ok := ctx.Item.(tracingv5.TaskStart); ok {
//	            // Process task start event
//	        }
//	    }
//	}
//
//	component.AcceptHook(&MyHook{})
package tracingv5
