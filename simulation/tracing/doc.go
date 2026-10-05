// Package tracing provides a task-based framework for observing what happens
// inside a simulation. It is built on the hooking mechanism, so components emit
// structured trace events without depending on any particular tracer.
//
// # Model
//
// A [Task] is the aggregate record a stateful tracer builds from a stream of
// events: it has a start and end time, a location (a kind-qualified component
// path — one location holds one kind, e.g. "L1.req_in", "L1.Top.incoming"), an
// optional parent, and lists of [TaskTag]s (categorical labels) and
// [Milestone]s (notable points, such as the resolution of a blocking condition).
// Components never build a Task directly — they emit lightweight event structs
// and the emit functions stamp the time from the domain clock and fire a hook.
//
// # Emitting
//
// The domain (a [NamedHookable]) is always the first argument; callers pass no
// time. Low-level: [StartTask], [EndTask], [AddTaskTag], [AddMilestone]. For
// request/response messages, the [TraceReqInitiate], [TraceReqReceive],
// [TraceReqComplete], and [TraceReqFinalize] helpers open and close the nested
// req_out/req_in tasks. A component that drops in-flight work on reset ends the
// open tasks with [EndReqInOnReset] and [EndTaskOnReset].
//
// # Collecting
//
// Attach a [Tracer] to a domain with [CollectTrace]; attach incoming- and
// outgoing-buffer tracing to a port with [CollectIncomingBufferTrace] and
// [CollectOutgoingBufferTrace]. Built-in tracers include [AverageTimeTracer],
// [TotalTimeTracer], [BusyTimeTracer], [TagCountTracer], [BackTraceTracer], and
// [DBTracer] (which persists to a DataRecorder).
//
// See README.md in this package for a fuller guide, including the milestone
// kinds, the buffer/req_in retrieve boundary, pipeline subtasks, and the
// DBTracer schema.
package tracing
