# Component System

A Component is an element in a simulator that maintains its own state. A typical component also handles events (see [The Event System](./The Event System)) and send/receive requests through ports and connections (see [The Connection System](./The Connection System)). Examples of components can be a CPU core, a cache module, and a DRAM controller.

## Event Handling

A Component handles events by the `Handle` function. When the event-driven simulation engine triggers an event, the engine calls the `Handle` function of the component, passing the event triggered as an argument.

A typical `Handle` function looks like this:

```go
func (c *ExampleComponent) Handle(e *Event) error {
    ctx := HookCtx{
        Domain: c,
        Now: evt.Time(),
        Item: evt,
        Pos: akita.HookPosBeforeEvent
    }
    cu.InvokeHook(&ctx)
    c.Lock()

    switch e := e.(type) {
    case akita.TickEvent:
        c.handleTickEvent(e)
    // ... Handle other event types:
    default:
        log.Panicf("cannot handle event of type %s", reflect.TypeOf(e))
    }

    c.Unlock()

    ctx.Pos = akita.HookPosAfterEvent
    cu.InvokeHook(&ctx)

    return nil
}
```

The main part of the handle function is a type switch, which is a feature of Go. A component can use the type switch to tell what type of event it is handling and to perform different actions. We also lock the whole `Handle` function with a mutex to eliminate data race.

An important rule is that: **one component can only schedule events for itself**. We use this rule the avoid cross-dependencies between components.

## Request Handling

The only proper way for 2 components to interact is to send requests via a connection. The connection will deliver requests into a port of a component. When there is a request arrives at a port, the port notifies the component with the `NotifyRecv` function. The component can either retrieve the request immediately or schedule an event to retrieve the request from the port and process the request later. In addition, when a port can accept a requenst to be sent from the component, the port calls the components `NotifyPortAvailabe` function. For details of connections and ports, please see [The Connection System](The Connection System).

Usually, a component should define the protocol it accepts alongside the implementation of the component.

We show two types of compoenents with the following examples.

## Example 1: Responder Components

Responder Components represent the simplest component type. It is useful for the Components that have very simple logic. An `IdealMemoryController` is a good example of a Responder Component, where a memory access response is generated after a certain number of cycles after the memory access request is received.

When a request arrives at a Responder Component, it schedules an event after a certain number of cycles. For the same example of the `IdealMemoryController`, when it receives a Read Request, in the `NotifyRecv` function, it immediately extracts the request from the port and schedules a `RespondDataReadyEvent` after a predefined number of cycles.

The `IdealMemoryController` handles the `RespondDataReadyEvent` in the `Handle` function. While handling the `RespondDataReadyEvent`, it reads the data from the underlying storage, assembles the `DataReadyRsp`, and send the response out. If the port cannot send the response out at this moment, the `IdealMemoryController` has to retry in the next cycle by scheduling another `RespondDataReadyEvent`. Note that this retrying process causes busy ticking and slows down the simulation. The Ticking Components may reduce the performance penalty.

## Example 2: Ticking Components

It is highly recommended to always use a Ticking Component model. It is simple, but can still model complex components with high performance. To define a component as a `TickingComponent`, one can simply composite the `TickingComponent` class inside the customized component.

TickingComponent defines both the `NotifyRecv` function and the `NotifyPortAvailable` functions so that a customized ticking component does not need to reimplement those functions. These two functions are implemented by simply schedules a `TickEvent` at the cycle right after `NotifyRecv` or `NotifyPortAvailable` is called.

A ticking component should always handle TickEvent. Note that if you are writing a type switch to find out the type of the event, `TickEvent` always use value type rather than the pointer type to save garbage collection time. A typical implementation of handling a `TickEvent` is like this:

```go
func (c *CustomizedComponent) Handle(evt akita.Event) {
    switch evt := evt.(type) {
    case akita.TickEvent:
        c.tick(evt.Time())
    // Other types of events
    default:
        panic("unsupported event type");
    }
}


func (c *CustomizedComponent) tick(now akita.VTimeInSec) {
    c.NeedTick = false

    ... // Perform the logic
    // While performing the logic, if any progress is made
    // set the NeedTick field to true

    if c.NeedTick {
        c.TickLater(now)
    }
}
```

A typical approach to handle request sending is to use a request buffer and a request sending stage. For example, if we rewrite the `IdealMemoryController` as a ticking component, we can add an extra field `SendToTopBuf []akita.Req`. We should also set the capacity of this buffer so that we forbid pushing too many elements into this buffer. Since we have the `SendToTopBuf` buffer, all other parts of the component can insert requests into this buffer if they need to communicate with the top component. We can centralize the request sending logic like the following:

```go
func (c *IdealMemoryController) sendToTop(now akita.VTimeInSec) {
    if len(c.SendToTopBuf) == 0 {
        return
    }

    req := c.SendToToBuf[0]
    req.SetSendTime(now)
    err := c.ToTop.Send(req)
    if err == nil {
        c.SendToTopBuf = c.SendToTopBuf[1:]
        c.NeedTick = true
    }
}
```

Using the `NeedTick` field, a ticking component never do busy ticking anymore. In any case that there is no progress can be made, the component must either: 1) be idling, or 2) having all out-going ports busy. If no progress can be made in one cycle, no further progress can be made until the component receives a new request, or one of the out-going port becomes idle. Therefore, since we schedules `TickEvent` when we are notified that a new request arrives (`NotifyRecv`) or a port gets free (`NotifyPortAvailable`), we never miss any opportunity to make progress nor keep repeating to retry useless operations.
