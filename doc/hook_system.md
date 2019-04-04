# The Hook System

A hook is a piece of code that reads or alters the simulation state. A hook applies to a hookable (e.g. an event-driven simulation engine, a component, or a connection). The hookable object invokes the hooks during simulation. Applying hooks can be achieved with the `AcceptHook` function like this:

```go
aHookable.AcceptHook(aHook)
```

## Implementing Hookables

A hookable is anything that implements the `Hookable` interface, including the `AcceptHook` and the `InvokeHook` functions. However, in most of cases, a hookable implementation can simply embed the HookableBase class to expose these two function implementations and  users do not need to implement these two functions. Here is an example of how to write a hookable class.

```go
type SomeHookableComponent struct {
    HookableBase

    ... // other fields
}
```

A hookable needs to call hooks explicitly. For example, when you write a component, you probably want to invoke hooks before and after handling an event. So you may want to implement the `Handle` function like the following:

```go
func (c *SomeHookableComponent) Handle(evt akita.Event) {
    ctx := HookCtx{
        Domain: c,
        Now: evt.Time(),
        Item: evt,
        Pos: akita.HookPosBeforeEvent
    }
    c.InvokeHook(&ctx)

    ... // Handle the event.

    ctx.Pos = akita.HookPosAfterEvent
    c.InvokeHook(&ctx)
}
```

In this example, we see that before the component handles the event, it calles the `InvokeHook` function with an argument called `ctx`. When this function is called, all the hooks that attach to the hookable object will be triggered. The hookable object invokes hooks, providing the `HookCtx` object that carries all the information related to the point that the hook is invoked. The `Domain` field in the `HookCtx` struct is the hookable object. The `Now` field is the virtual time that the hook is called. Finally, the `Pos` and the `Item` is related to the reason why the hook is called.

Examples of `HookPos` include `HookPosBeforeEvent` and `HookPosAfterEvent` , which are to be used before and after the event handling, respectively. Users can also define customized hook position as a global variable as follows.

```go
var InstReadStartHookPos = &HookPos{"InstReadStart"}
```

## Implementing Hooks

Implemenging the hook involves implementing the `Func` function. Generally, the `Func` function first check the `Item` and the `Pos` fields in the `HookCtx` to see if the hook is interested in the triggering point. Then, the rest of the `Func` function defines what action that the hook performs. For example, logger hooks may write the information into a log file a tracer hook may insert a record into a database and a fault injection hook may alter the value of a register.

## Predefined Hook

Project Akita and each simulator model provide predefined hooks. In project Akita, we provide the `EventLogger` hook. It can be applied to any Component or any event-driven simulation engine. If it is hooked to an event-driven simulation engine, it logs all the events triggered. If it hooks to a Component, it logs the events handled by the component.
