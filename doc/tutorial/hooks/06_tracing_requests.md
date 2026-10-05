---
sidebar_position: 6
---

# Tracing Requests

A single-component task has one obvious start and end. But the most common
thing you want to measure spans **two** components: a request is sent, the
other side handles it, and a response comes back. That involves two related
tasks — one on the sender, one on the receiver — and four moments to mark.
Akita gives you four helpers for exactly this, so you do not hand-roll the
ids and parent links.

The example is in `examples/reqtracing/`.

## What You Will Learn

- The four stages of a request: initiate, receive, complete, finalize.
- Which side calls which helper, and the two tasks they create.
- How to read round-trip latency and handling time from the same run.
- How `MsgIDAtReceiver` chains tasks across components into a task tree.

## The Four Stages

A request creates two nested tasks:

| Stage | Helper | Called by | Effect |
|---|---|---|---|
| Initiate | `TraceReqInitiate(domain, req, parentID)` | sender, on send | opens a `req_out` task (the round trip) |
| Receive (start) | `TraceReqReceive(domain, req)` | receiver, on arrival | opens a `req_in` task (handling), child of `req_out` |
| Complete (end) | `TraceReqComplete(domain, req)` | receiver, when done | ends the `req_in` task |
| Finalize | `TraceReqFinalize(domain, req)` | sender, on response | ends the `req_out` task |

Every helper takes the **domain first, then the message** (and `parentID`
last, for `TraceReqInitiate`).

The `req_out` task spans the whole round trip as the sender sees it; the
`req_in` task spans just the receiver's handling and is recorded as a child.
The two tasks agree on identity without you managing ids by hand: the
`req_out` task is keyed by the request's own message ID (`req.Meta().ID`), and
`TraceReqReceive` opens its `req_in` task with that same ID as the parent. The
receiver-side task gets its own id from a tracing-local registry — the message
value itself is never mutated, so there is no `SendTaskID`/`RecvTaskID` field
to thread through.

The one thing you must do is **hold onto the original request** on each side:
the sender finalizes with the request it sent, and the receiver completes
with the request it received. Both sides keep it in their `State`, because a
request in flight is runtime data that must survive a checkpoint.

The client and the server are two ticking components, each in its own
package (`reqtracing/client` and `reqtracing/server`); the `ReadReq` and
`ReadRsp` messages live in `server`. `main.go` is the system builder that
wires them together.

## Sender Side

The client sends one request at a time. It initiates the task just before
sending, then keeps the request in its State's `InFlight` map until the
response arrives:

```go
func (m *requestMW) send() bool {
    s := &m.comp.State
    spec := m.comp.Spec
    port := m.comp.Ports.Out

    // Send one request at a time: wait for the response before the next.
    if s.NextSeq == spec.NumReqs || len(s.InFlight) > 0 || !port.CanSend() {
        return false
    }

    req := server.ReadReq{
        MsgMeta: messaging.MsgMeta{
            ID:  m.comp.NewID(),
            Src: port.AsRemote(),
            Dst: spec.Dst,
        },
        Seq: s.NextSeq,
    }

    // The req_out task is keyed by the request's own message ID.
    tracing.TraceReqInitiate(m.comp, req, 0)
    port.Send(req)

    s.InFlight[req.ID] = req
    s.NextSeq++

    return true
}

func (m *requestMW) receive() bool {
    s := &m.comp.State
    port := m.comp.Ports.Out

    msg, ok := port.PeekIncoming()
    if !ok {
        return false
    }

    rsp := msg.(server.ReadRsp)
    if req, ok := s.InFlight[rsp.RspTo]; ok {
        tracing.TraceReqFinalize(m.comp, req)
        delete(s.InFlight, rsp.RspTo)
    }
    port.RetrieveIncoming()

    return true
}
```

Messages are value types, and `port.Send` returns nothing — the `CanSend()`
check above is what guarantees there is room. `TraceReqInitiate` runs before
`Send` so the `req_out` task exists by the time anything observes the message.
When the response comes back, the client looks up the original request by
`rsp.RspTo` and finalizes with it. (`InFlight` starts as an empty map: the
client's `Definition` has a `NewState` that creates it.)

## Receiver Side

The server opens the handling task when it picks up the request, counts down
a fixed latency, then completes the task and sends the response:

```go
func (m *serveMW) receive() bool {
    port := m.comp.Ports.Out

    msg, ok := port.PeekIncoming()
    if !ok {
        return false
    }

    req := msg.(ReadReq)
    tracing.TraceReqReceive(m.comp, req)
    m.comp.State.Pending = append(m.comp.State.Pending,
        txn{Req: req, Left: m.comp.Spec.Latency})
    port.RetrieveIncoming()

    return true
}

func (m *serveMW) respond() bool {
    s := &m.comp.State
    if len(s.Pending) == 0 || s.Pending[0].Left > 0 {
        return false
    }

    port := m.comp.Ports.Out
    if !port.CanSend() {
        return false
    }

    req := s.Pending[0].Req
    port.Send(ReadRsp{
        MsgMeta: messaging.MsgMeta{
            ID:    m.comp.NewID(),
            Src:   port.AsRemote(),
            Dst:   req.Src,
            RspTo: req.ID,
        },
        Seq: req.Seq,
    })

    tracing.TraceReqComplete(m.comp, req)
    s.Pending = s.Pending[1:]

    return true
}
```

## Measuring Both Tasks

Because the two tasks have different kinds (`req_out` and `req_in`), one
filter picks out each. We attach an `AverageTimeTracer` to each side:

```go
roundTrip := tracing.NewAverageTimeTracer(
    func(t tracing.TaskStart) bool { return t.Kind == "req_out" })
handling := tracing.NewAverageTimeTracer(
    func(t tracing.TaskStart) bool { return t.Kind == "req_in" })
tracing.CollectTrace(cli, roundTrip)
tracing.CollectTrace(srv, handling)
```

The filter is a `func(tracing.TaskStart) bool` — it inspects the task at the
moment it starts. `NewAverageTimeTracer` reads the clock from the domain it is
attached to, so the constructor takes only the filter.

## Running It

```bash
cd examples/reqtracing
go run main.go
```

Output:

```
requests completed:           3
avg round trip (req_out):     7000 ps
avg server handling (req_in): 5000 ps
```

The server handles each request in 5000 ps; the full round trip is 7000 ps.
The extra 2000 ps is the two trips across the connection — visible precisely
because `req_out` wraps `req_in`. The same run yields both numbers; the
filter is what decides which task each tracer measures.

## Chaining Tasks Across Components

Real work rarely stops at one component. An L1 cache that misses sends a
request to L2; if L2 also misses, it goes to memory. Each hop is its own
request with its own `req_out`/`req_in` pair — and you want them linked, so
the trace shows the L2 access happened *because of* the L1 miss.

The link is the `parentID` you pass to `TraceReqInitiate`. When a component
fires off a downstream request *while handling* an upstream one, the
downstream task's parent should be the upstream **handling task**. But what
is that task's id?

### `MsgIDAtReceiver`

When you called `TraceReqReceive(comp, req)`, it opened a `req_in` task whose
id is the request's receiver-side id. That id is not stored on the message; it
lives in a tracing-local registry keyed by `(domain, req.Meta().ID)`. You read
back the same id with:

```go
parentID := tracing.MsgIDAtReceiver(upReq, comp)
```

Note the argument order: `MsgIDAtReceiver` takes the **message first, then the
domain**. So a cache that misses initiates its downstream request as a child
of the task it is currently handling. From the cache's middleware in
`examples/tasktree/cache`:

```go
// Open the handling task for the request we received.
tracing.TraceReqReceive(m.comp, upReq) // req_in @ this cache

// Miss: send a request one level down, parented to the task above.
downReq := memory.NewReq(m.comp.NewID(),
    bottom.AsRemote(), m.comp.Spec.Downstream)
tracing.TraceReqInitiate(m.comp, downReq,
    tracing.MsgIDAtReceiver(upReq, m.comp))
bottom.Send(downReq)
```

`TraceReqReceive` on the next component down then parents *its* `req_in` to
this `downReq`'s `req_out` automatically (the parent is just `downReq`'s
message ID). Apply the pattern at every level and the parent links chain all
the way down.

### The Task Tree

`examples/tasktree` wires a client to a small hierarchy — `Client → L1 → L2 →
Memory` — where each cache misses and forwards downward using exactly that
pattern. `L1` and `L2` are two instances of the one component in the `cache`
package; the system builder builds the hierarchy bottom-up, so each cache's
`Spec.Downstream` can name the `Top` port of the level below. A custom tracer
attached to every component records each task's kind, parent, and location,
then prints them by parent link:

```
req_out @ Client.req_out
  req_in @ L1.req_in
    req_out @ L1.req_out
      req_in @ L2.req_in
        req_out @ L2.req_out
          req_in @ Memory.req_in
```

(Each location is suffixed with the task's kind: a component's `req_in`/`req_out`
tasks live at `<component>.req_in`/`.req_out`. This "one location, one kind" scheme
is what lets Daisen group a component's tasks — see chapter 7.)

That tree is the whole story of one request: the client's outbound task
parents L1's handling task, which parents L1's downstream task, and so on
down to the leaf task at memory. The example attaches **one** tracer instance
to all four components — `CollectTrace` is happy to put the same tracer on
many domains — which is how it sees the complete tree. With a `DBTracer` (next
chapter) the same tree is recorded to a database, so you can later ask where
a request spent its time across the entire hierarchy.

## Key Concepts

- **A request is two nested tasks**: `req_out` (round trip, sender) parents
  `req_in` (handling, receiver).
- **Four helpers mark the four moments** — initiate/finalize on the sender,
  receive/complete on the receiver — each taking the domain first, then the
  message. The `req_out` task is keyed by the message ID; the receiver-side id
  comes from a tracing-local registry, not the message.
- **Hold the original request** on each side: finalize and complete take the
  message they began with.
- **Filter by kind** to separate round-trip latency from handling time.
- **Chain tasks with `parentID`.** A downstream request initiated while
  handling an upstream one uses `tracing.MsgIDAtReceiver(upReq, comp)` as its
  parent, so requests across a component hierarchy form one task tree.

## Where to Next

We have used a couple of tracers in passing. The next chapter surveys all the
built-in tracers and goes deep on the **filter** — the function that selects
which tasks a tracer measures.
