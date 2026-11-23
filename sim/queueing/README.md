# queueing — Generic Buffers and Pipelines

Package `queueing` provides generic `Buffer[T]`, `Pipeline[T]`, and
`FastPipeline[T]` data structures for the Akita simulation framework. They are
reusable building blocks for component state: a bounded buffer with FIFO and
indexed access, a fixed-latency, multi-lane pipeline that a component embeds in
its `State` and drives once per tick, and a pull-based pipeline that jumps over
elapsed cycles so its component can sleep.

## How It Works

Both types fully encapsulate their state behind methods — their fields are
unexported, so callers interact through the API only, which keeps the capacity
and remaining element order intact. Construct them with the provided constructors,
which return values so they can be embedded directly in a component's state.

A `Pipeline[T]` drains its completed items into a `Sink[T]`, which is any
destination that can accept an item:

```go
type Sink[T any] interface {
    CanPush() bool
    Push(T)
}
```

A `*Buffer[T]` satisfies `Sink[T]`, so a buffer is the natural post-pipeline
landing spot.

## Buffer[T]

A bounded buffer with FIFO and indexed access. Create one with `MakeBuffer`:

```go
inbox := queueing.MakeBuffer[MyRequest](16)

if inbox.CanPush() {
    inbox.Push(req)
}

fmt.Println(inbox.Size(), inbox.Capacity())
```

### Key Methods

| Method | Description |
|--------|-------------|
| `CanPush() bool` | True if the buffer has room for another element |
| `Push(e T)` | Add an element to the back (panics if full) |
| `Peek() (T, bool)` | Read the head without removing it |
| `Pop() (T, bool)` | Remove and return the head |
| `PeekAt(index int) (T, bool)` | Read an entry by index; the head is index 0 |
| `PopAt(index int) (T, bool)` | Remove an indexed entry, preserving remaining order |
| `UpdateFront(e T) bool` | Replace the head; false if empty |
| `Clear()` | Remove all elements |
| `Size() int` | Current number of elements |
| `Capacity() int` | Maximum capacity |

Reads return the zero value and `false` when empty or out of range, including
negative indices. A stored zero or nil value returns `true`. `PopAt` shifts later
indices down by one. Invalid removals do not mutate the buffer.
Head removal is O(1); indexed removal shifts the entries after the removed item.

Buffers and pipelines require serialized access. `CanPush`, `CanAccept`, and
peek operations do not reserve capacity or entries.

`FastPipeline[T]` shares the `CanPush`, `Push`, `Peek`, and `Pop` names, so moving
between the two reads the same at the call site.

### Checkpointing

`Buffer[T]`, `Pipeline[T]`, and `FastPipeline[T]` implement
`MarshalJSON`/`UnmarshalJSON`, so their
full contents are captured when a component's `State` is checkpointed — no extra
work is needed as long as the element type `T` itself serializes. A
`FastPipeline`'s clock is the one exception: it is not saved, and loading decodes
State into a fresh value, so the component rebinds it:

```go
func (s *state) AfterCheckpointLoad(c *Comp) {
    s.Pipeline.BindClock(c)
}
```

For background on making a component, message, or event checkpointable, see
[`doc/tutorial/checkpointing.md`](../../doc/tutorial/checkpointing.md).

## Pipeline[T]

**Deprecated:** use [`FastPipeline[T]`](#fastpipelinet). `Pipeline[T]` and
`MakePipeline` remain available for existing models, and no Akita component uses
them. To migrate a `Pipeline` that drains into a post-pipeline buffer, construct
it with the owner's frequency and clock, replace `pipe.Tick(&buf)` with
`pipe.DrainReady(&buf)`, rename `CanAccept`/`Accept` to `CanPush`/`Push`, rebind
the clock in `AfterCheckpointLoad`, and call `TickAt` with `NextReadyTime`. The
buffer keeps its capacity. Items that complete in the same cycle now enter it
oldest first, and a component that sleeps no longer runs per-tick work on the
cycles it skips.

A multi-lane, multi-stage pipeline that models fixed-latency processing. Items
enter at stage 0 and advance one stage per tick until they exit the last stage
into a `Sink[T]`. Total latency through the pipeline equals `numStages` ticks.
Create one with `MakePipeline`:

```go
pipe := queueing.MakePipeline[MyItem](4, 3) // 4 lanes, 3 stages
post := queueing.MakeBuffer[MyItem](8)

if pipe.CanAccept() {
    pipe.Accept(item)
}

madeProgress := pipe.Tick(&post)
```

### Key Methods

| Method | Description |
|--------|-------------|
| `CanAccept() bool` | True if a lane is free at stage 0 |
| `Accept(item T)` | Insert into the first stage; panic if full |
| `Tick(sink Sink[T]) bool` | Advance one cycle; completed items go to `sink`. Returns true if any item moved |
| `Stages() []PipelineStage[T]` | Copy of current occupancy, for inspection and testing |
| `Clear()` | Remove all items |

### Pipeline Behavior

- Each item occupies one lane at one stage, described by `PipelineStage[T]`
  (`Lane`, `Stage`, `Item`).
- Items advance from stage `i` to stage `i+1` when the same lane is free at the
  next stage.
- Items at the last stage are pushed into the sink, but
  only while `sink.CanPush()` is true.
- Stages are processed from last to first to prevent double-advancement within
  a tick.

## FastPipeline[T]

A multi-lane, multi-stage pipeline embedded by value in component state, with
no component, builder, or Spec of its own and no sink. It reads time from an
injected read-only clock, ordinarily its owner, and items catch up lazily at a
cost proportional to the resident items, so a component can sleep until the next
cycle that matters. The pipeline never schedules events: its owner asks when to
come back and calls `TickAt`.

```go
// In State.Initialize: 4 lanes, 3 stages, the owner's frequency and clock.
s.Pipe = queueing.MakeFastPipeline[MyItem](4, 3, c.Spec.Freq, c)

// In each tick, in this order:
pipe.UpdateReady(complete) // idempotent work on every ready lane head

for output.CanSend() {
    item, ok := pipe.Peek()
    if !ok {
        break
    }
    output.Send(item)
    pipe.Pop() // only after the output accepted the item
}

if pipe.CanPush() {
    pipe.Push(next)
}

if due, ok := pipe.NextReadyTime(); ok {
    c.TickAt(due)
}
```

### Key Methods

| Method | Description |
|--------|-------------|
| `CanPush() bool` | True if a lane has stage 0 free in this cycle |
| `Push(item T)` | Insert at stage 0 of the lowest free lane; panics if none |
| `Peek() (T, bool)` | Read the oldest pushed ready item |
| `Pop() (T, bool)` | Remove the oldest pushed ready item |
| `UpdateReady(update func(T) T)` | Apply idempotent work to every ready lane head, oldest first, keeping results attached while blocked |
| `DrainReady(sink Sink[T]) bool` | Move ready heads into a sink, oldest first, while it has room; true if any moved |
| `NextReadyTime() (timing.VTimeInPicoSec, bool)` | Earliest future clock edge at which a lane head becomes ready, assuming no further pops; already-ready heads are excluded |
| `NextPushTime() (timing.VTimeInPicoSec, bool)` | Earliest time `CanPush` holds without a pop; false when all lanes are full |
| `Pause()` / `Resume()` | Stop item progress at the end of the previous cycle; resume with the current cycle as the first that counts. Both deadlines report false while paused |
| `BindClock(clock timing.TimeTeller)` | Reattach the clock after a checkpoint load |
| `Stages() []PipelineStage[T]` | Copy of occupancy with lane identity, oldest pushed first; does not change the pipeline |
| `Size() int` / `Clear()` | Resident count; remove all items, keeping the clock position and any pause |

### Timing

- An item pushed in cycle `c` is ready in cycle `c + numStages`, the same
  latency as `Pipeline`. Every call on one clock edge sees the same cycle.
- A clock moving backward panics.
- Each lane is an independent FIFO that holds at most `numStages` items, and
  delivers and accepts at most one item per cycle. A blocked lane does not hold
  back another lane.
- A pop releases its lane in the same cycle: the items behind it close the gap
  by one stage, so a full lane can accept in that cycle. An item that already
  moved in this cycle does not move again, and the next item in the lane is not
  ready until the next cycle.
- `Peek` and `Pop` pick the oldest pushed ready head, which makes cross-lane
  order deterministic. Lanes are private and only `Stages` reports them.
- Unlike a `Pipeline` draining into a `Buffer`, there is no separate output
  queue unless the component keeps one with `DrainReady`: the pipeline's own
  capacity is exactly `width * numStages`.
- `numStages` must be positive. A zero-latency stage bypasses its pipeline.
- A push `Pipeline` snapshot cannot be restored into a `FastPipeline`. Loading
  one reports an error rather than dropping its in-flight items.
