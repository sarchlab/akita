# Tasks and generic hooks

Task observers subscribe to the simulation's Tracker. Components and ports call
`tasks.StartTask`, `tasks.EndTask`, and the other typed commands directly. The Tracker
resolves parent correlation and notifies observers with `TaskStarted`,
`TaskEnded`, `TagAdded`, and `MilestoneAdded`.

`twowaybuffered.Port` is a task owner. It retains buffer task IDs and wait
boundaries beside its messages and ends tasks on retrieval. Components can call
`port.AddIncomingMilestone(tasks.Reason(kind, what))` before retrieval or at a
wait-stage transition; connections have the symmetric `AddOutgoingMilestone`.
No tracker hook or adapter is needed.

Generic port hooks remain available. They carry the affected message in Item,
with nil Detail. Hooks execute under the port lock: do not call Peek, Retrieve,
Send, or count methods from a callback. The locking order is port, tracker,
observer. Task observers must not re-enter either earlier layer. See the
[task guide](/packages/tasks/) for concurrency and subscription scope.
