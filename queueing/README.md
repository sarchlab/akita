# queueing — Generic Buffers and Pipelines

Package `queueing` provides generic `Buffer[T]` and `Pipeline[T]` data
structures for the Akita simulation framework. They are reusable building
blocks for component state: a bounded buffer with FIFO and indexed access, and a fixed-latency,
multi-lane pipeline that a component embeds in its `State` and drives once per
tick.

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

A bounded buffer with FIFO and indexed access and hook support. Create one with `NewBuffer`:

```go
inbox := queueing.NewBuffer[MyRequest]("inbox", 16)

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
| `Name() string` | Buffer name (for hooks and monitoring) |

Reads return the zero value and `false` when empty or out of range, including
negative indices. A stored zero or nil value returns `true`. `PopAt` shifts later
indices down by one. Invalid removals do not mutate the buffer or fire hooks.
Head removal is O(1); indexed removal shifts the entries after the removed item.

Buffers and pipelines require serialized access. `CanPush`, `CanAccept`, and
peek operations do not reserve capacity or entries.

### Hook Positions

`Buffer[T]` embeds `hooking.HookableBase` and fires:

- `HookPosBufPush` — after an element is pushed.
- `HookPosBufPop` — after an element is popped.

### Checkpointing

`Buffer[T]` and `Pipeline[T]` implement `MarshalJSON`/`UnmarshalJSON`, so their
full contents are captured when a component's `State` is checkpointed — no extra
work is needed as long as the element type `T` itself serializes. For background
on making a component, message, or event checkpointable, see
[`doc/tutorial/checkpointing.md`](../doc/tutorial/checkpointing.md).

## Pipeline[T]

A multi-lane, multi-stage pipeline that models fixed-latency processing. Items
enter at stage 0 and advance one stage per tick until they exit the last stage
into a `Sink[T]`. Total latency through the pipeline equals `numStages` ticks.
Create one with `NewPipeline`:

```go
pipe := queueing.NewPipeline[MyItem](4, 3) // 4 lanes, 3 stages
post := queueing.NewBuffer[MyItem]("post", 8)

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
| `AcceptWithDelay(item T, delay int)` | Like `Accept` (including panic if full), but the item dwells `delay` extra cycles at stage 0 |
| `Tick(sink Sink[T]) bool` | Advance one cycle; completed items go to `sink`. Returns true if any item moved |
| `Stages() []PipelineStage[T]` | Copy of current occupancy, for inspection and testing |
| `Clear()` | Remove all items |

### Pipeline Behavior

- Each item occupies one lane at one stage, described by `PipelineStage[T]`
  (`Lane`, `Stage`, `Item`, `CycleLeft`).
- Items advance from stage `i` to stage `i+1` when the same lane is free at the
  next stage.
- Items at the last stage with `CycleLeft == 0` are pushed into the sink, but
  only while `sink.CanPush()` is true.
- Stages are processed from last to first to prevent double-advancement within
  a tick.
