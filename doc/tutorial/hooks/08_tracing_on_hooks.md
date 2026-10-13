# Tasks and generic hooks

Task observers subscribe to the simulation's Tracker. Component commands such as
`tasks.Start` call this typed service directly; they do not emit generic hooks.
The Tracker owns the open records and parent correlation, then notifies observers
with `TaskStarted`, `TaskEnded`, `TagAdded`, and `MilestoneAdded`.

Ports remain generic messaging objects. Their send, receive, and retrieve hooks
carry the affected message and a `PortBufferChange` describing the actual head.
When tracking is enabled, the simulation registers the Tracker's port adapter.
It owns buffer task IDs keyed by port and message, and converts these facts into
buffer starts/ends and queue milestones.

This adapter performs buffer lifecycle work; ordinary task notifications need
no dispatch wrapper. A custom generic Hook can still observe other component
or engine events independently of task tracking.

Port hooks execute under the port lock. Read their supplied head facts; do not
call Peek, Retrieve, Send, or count methods from a callback. The locking order is
port, tracker, observer. Task observers must not re-enter either earlier layer.
See [the task guide](../../../sim/tasks/README.md) for concurrency and scope.
