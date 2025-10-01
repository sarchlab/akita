// Package tracingv5 provides hook-based task tracing infrastructure.
//
// This package defines hook positions and data structures for tracing task
// execution in simulations. Unlike the legacy tracing package which provides
// tracer APIs, tracingv5 relies entirely on the hooking infrastructure for
// extensibility.
//
// # Hook Positions
//
// The package defines four hook positions:
//   - HookPosTaskStart: Triggered when a task begins execution
//   - HookPosTaskEnd: Triggered when a task completes
//   - HookPosTaskTag: Triggered when metadata is added to a task
//   - HookPosTaskStep: Triggered when a task reaches a milestone
//
// # Event Structures
//
// Each hook position has a corresponding event structure:
//   - TaskStart: Contains task ID, timing, location, and parent information
//   - TaskEnd: Contains task ID, timing, and location
//   - TaskTag: Contains task ID, timing, and tag value
//   - TaskStep: Contains task ID, timing, step name, and step ID
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
//	        Time:  engine.CurrentTime(),
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
