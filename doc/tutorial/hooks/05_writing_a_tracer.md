# Writing a task observer

Embed `tasks.NopObserver` to implement only the callbacks you need. A duration
observer needs only `TaskEnded`: the completed Task contains both timestamps.

```go
type maxDuration struct {
    tasks.NopObserver
    mu sync.Mutex
    maximum timing.VTimeInPicoSec
}
func (o *maxDuration) TaskEnded(t tasks.Task) {
    o.mu.Lock()
    defer o.mu.Unlock()
    if duration := t.EndTime - t.StartTime; duration > o.maximum {
        o.maximum = duration
    }
}
```

Register it with `s.TaskTracker().Subscribe(observer, worker)` before the first run
to observe starts. Late subscriptions receive future events, including complete
ends for in-flight tasks, without history replay.
Omit the owner arguments to observe the whole simulation, including port tasks.
The tracker supplies resolved parent IDs consistently to every observer.

The other callbacks are `TaskStarted(Task)`, `TagAdded(Tag)`, and
`MilestoneAdded(Milestone)`. Different tasks may invoke one observer concurrently,
so synchronize both updates and accessors. Callbacks must not call into the
tracker or port buffers. They may retain event values, but must treat `Detail`
as borrowed read-only data.

See [examples/customtracer](https://github.com/sarchlab/akita/tree/main/examples/customtracer) for a complete
example. No Hook adapter or per-observer request-parent registry is needed.
