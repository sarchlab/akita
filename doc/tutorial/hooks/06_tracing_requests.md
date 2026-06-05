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

## The Four Stages

A request creates two nested tasks:

| Stage | Helper | Called by | Effect |
|---|---|---|---|
| Initiate | `TraceReqInitiate(req, domain, parentID)` | sender, on send | opens a `req_out` task (the round trip) |
| Receive (start) | `TraceReqReceive(req, domain)` | receiver, on arrival | opens a `req_in` task (handling), child of `req_out` |
| Complete (end) | `TraceReqComplete(req, domain)` | receiver, when done | ends the `req_in` task |
| Finalize | `TraceReqFinalize(req, domain)` | sender, on response | ends the `req_out` task |

The `req_out` task spans the whole round trip as the sender sees it; the
`req_in` task spans just the receiver's handling and is recorded as a child.
The helpers thread the ids through the message's metadata for you
(`SendTaskID` on the way out, `RecvTaskID` at the receiver), so the two sides
agree without you managing ids by hand.

The one thing you must do is **hold onto the original request** on each side:
the sender finalizes with the request it sent, and the receiver completes
with the request it received.

## Sender Side

The client sends one request at a time. It initiates the task just before
sending, then keeps the request in an in-flight map until the response
arrives:

```go
func (m *clientMW) send() bool {
    s := &m.comp.State
    port := m.comp.GetPortByName("Out")

    if s.ReqsToSend == 0 || len(m.inFlight) > 0 || !port.CanSend() {
        return false
    }

    req := &readReq{ /* MsgMeta{ID, Src, Dst}, Seq */ }

    tracing.TraceReqInitiate(req, m.comp, 0) // open req_out
    port.Send(req)
    m.inFlight[req.ID] = req

    s.ReqsToSend--
    s.NextSeq++
    return true
}

func (m *clientMW) receive() bool {
    rsp := m.comp.GetPortByName("Out").PeekIncoming().(*readRsp)
    if req, ok := m.inFlight[rsp.RspTo]; ok {
        tracing.TraceReqFinalize(req, m.comp) // close req_out
        delete(m.inFlight, rsp.RspTo)
    }
    m.comp.GetPortByName("Out").RetrieveIncoming()
    return true
}
```

`TraceReqInitiate` runs before `Send` so the request carries its `SendTaskID`
across the wire. When the response comes back, the client looks up the
original request by `rsp.RspTo` and finalizes with it.

## Receiver Side

The server opens the handling task when it picks up the request, counts down
a fixed latency, then completes the task and sends the response:

```go
func (m *serverMW) receive() bool {
    req := m.comp.GetPortByName("Out").PeekIncoming().(*readReq)
    tracing.TraceReqReceive(req, m.comp) // open req_in
    m.pending = append(m.pending, serverTxn{req: req, left: m.comp.Spec().Latency})
    m.comp.GetPortByName("Out").RetrieveIncoming()
    return true
}

func (m *serverMW) respond() bool {
    if len(m.pending) == 0 || m.pending[0].left > 0 {
        return false
    }
    txn := m.pending[0]
    rsp := &readRsp{ /* MsgMeta{ID, Src, Dst, RspTo: txn.req.ID}, Seq */ }
    if m.comp.GetPortByName("Out").Send(rsp) != nil {
        return false
    }
    tracing.TraceReqComplete(txn.req, m.comp) // close req_in
    m.pending = m.pending[1:]
    return true
}
```

## Measuring Both Tasks

Because the two tasks have different kinds (`req_out` and `req_in`), one
filter picks out each. We attach an `AverageTimeTracer` to each side:

```go
roundTrip := tracing.NewAverageTimeTracer(engine,
    func(t tracing.Task) bool { return t.Kind == "req_out" })
handling := tracing.NewAverageTimeTracer(engine,
    func(t tracing.Task) bool { return t.Kind == "req_in" })

tracing.CollectTrace(client, roundTrip)
tracing.CollectTrace(server, handling)
```

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

## Key Concepts

- **A request is two nested tasks**: `req_out` (round trip, sender) parents
  `req_in` (handling, receiver).
- **Four helpers mark the four moments** — initiate/finalize on the sender,
  receive/complete on the receiver — threading the ids through the message
  metadata for you.
- **Hold the original request** on each side: finalize and complete take the
  message they began with.
- **Filter by kind** to separate round-trip latency from handling time.

## Where to Next

We have used a couple of tracers in passing. The next chapter surveys all the
built-in tracers and goes deep on the **filter** — the function that selects
which tasks a tracer measures.
