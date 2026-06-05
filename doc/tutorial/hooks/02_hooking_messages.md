---
sidebar_position: 2
---

# Hooking into Messages

The engine is not the only `Hookable` object. **Ports** fire hooks too, so
you can watch the messages flowing between components — again without
touching the components. Same example program, `examples/hooks/`.

## What You Will Learn

- The port hook positions for sending and receiving messages.
- How to attach a hook to a specific port.
- How a hook can carry its own state.

## Port Hook Positions

A port publishes a hook position for each thing it does with a message. The
two we care about here:

```go
var HookPosPortMsgSend  = &hooking.HookPos{Name: "Port Msg Send"}
var HookPosPortMsgRecvd = &hooking.HookPos{Name: "Port Msg Recv"}
```

At both positions `ctx.Item` is the `messaging.Msg` involved. From there you
can type-switch on the concrete message type to inspect its payload, or read
its routing metadata with `msg.Meta()`.

## A Message-Logging Hook

This hook logs every message a port sends or receives. It also carries the
agent's name, which shows that a hook is an ordinary struct — it can hold
whatever state it needs:

```go
type msgHook struct {
    agent string
}

func (h *msgHook) Func(ctx hooking.HookCtx) {
    msg := ctx.Item.(messaging.Msg)

    switch ctx.Pos {
    case messaging.HookPosPortMsgSend:
        fmt.Printf("[msg]   %s sends %T\n", h.agent, msg)
    case messaging.HookPosPortMsgRecvd:
        fmt.Printf("[msg]   %s recvd %T\n", h.agent, msg)
    }
}
```

A port is reached by name and is itself `Hookable`, so we attach one hook
per agent's `Out` port:

```go
agentA.GetPortByName("Out").AcceptHook(&msgHook{agent: "AgentA"})
agentB.GetPortByName("Out").AcceptHook(&msgHook{agent: "AgentB"})
```

## Running It

With both the message hook and the event hook from the previous chapter
attached, the run shows the full conversation. Filtering to just the message
lines:

```
[msg]   AgentA sends *main.pingReq
[msg]   AgentB recvd *main.pingReq
[msg]   AgentB sends *main.pingRsp
[msg]   AgentA recvd *main.pingRsp
```

The whole round trip, observed from the ports: Agent A sends a request,
Agent B receives it, Agent B sends a response, Agent A receives it. The
agents' middleware is exactly the same code as before — the hooks just watch
the traffic go by.

## Key Concepts

- **Ports are `Hookable` too.** `HookPosPortMsgSend` and
  `HookPosPortMsgRecvd` fire as messages leave and arrive; `ctx.Item` is the
  `messaging.Msg`.
- **Attach per object.** `port.AcceptHook(...)` watches that one port; you
  decide which ports to observe.
- **A hook is just a struct.** It can carry state (here, the agent name) to
  enrich what it reports.

## Where to Next

So far you have attached hooks to points Akita provides — engine events and
port messages. The next chapter shows how to expose your component's *own*
internal behavior by **defining your own hook point**.
