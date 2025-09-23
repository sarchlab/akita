# Tracing

The tracing package builds on the shared [hooking](../hooking) primitives to
expose a structured, three-tier instrumentation story:

1. **Hooking (`instrumentation/hooking`)** – defines the reusable `Hookable`
   contract and `HookCtx` payload that simulation components use to publish
   events.
2. **Task-level tracing (`instrumentation/tracing`)** – wraps those hooks with
   task semantics, providing helpers such as `StartTask`, `AddTaskTag`,
   `AddMilestone`, and `EndTask` that populate the task/tag/milestone data
   structures before invoking the underlying hook.
3. **Request-level tracing** – composes the task helpers into canonical message
   lifecycles via `TraceReqInitiate`, `TraceReqReceive`, `TraceReqComplete`, and
   `TraceReqFinalize` so request/response components report consistent events.

Tasks now expose *tags* (renamed from "steps") to capture significant
checkpoints during processing. Tags flow through the same hook bus as other
tracing events, allowing tracers to correlate and aggregate activity without
manually managing hook contexts.
