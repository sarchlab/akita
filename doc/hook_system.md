A hook is a piece of code that reads or alters the simulation state. A hook applies to a hookable (can be an event-driven simulation engine, component, or a connection). During the simulation execution, the hookable calls the hooks when appropriate. 

How a hookable knows it is time to invoke a hook? It is based on the 2 pieces of information provided by the hook. The first is the hook type (function `Type`), indicating what triggers the hook. For example, if a hook has a hook type of `TickEvent`, the hook should be called when the hookable is handling a `TickEvent`. Furthermore, the hookable also need to know if the hook needs to be called before or after handling the event. Assuming we want the hook to be called after the event is handled, we need  the `Pos` function of the hook to return `AfterEventHookPos`. 

A simulator user needs to apply hooks to hookables when configuring the platform under simulation. Applying hooks can be achieved with the `AcceptHook` function like this:

```go
aHookable.AcceptHook(aHook)
```

Next, we will give detailed instruction on how to write hookables and hooks.

## Implementing Hookables

A hookable is anything that implements the `Hookable` interface. A `Hookable` interface defines two functions including the `AcceptHook` and the `InvokeHook` function. However, any hookable implementation can simply embed the HookableBase class to expose these two function implementations. In general, users do not need to implement these two functions. Here is an example of how to write a hookable class.

```go
type SomeHookableComponent struct {
    HookableBase
}
```

A hookable needs to call hooks explicitly. For example, when you write a component, you probably want to invoke hooks before and after handling an event. So you may want to implement the `Handle` function like the following:

```go
func (c *SomeHookableComponent) Handle(evt akita.Event) {
    c.InvokeHook(evt, c, akita.BeforeEventHookPos, nil)
    ... // Handle the event.
    c.InvokeHook(evt, c, akita.AfterEventHookPos, nil)
}
```

In this example, we see that before the component handles the event, it calls the `Invoke` hook function with 4 arguments. The first is the event itself, followed by the component that calls the hook. The hook needs these two arguments to run the hook logic. The third argument is the position of the hook. Combining with the first argument, the `InvokeHook` function can determine which hook to call. If multiple hooks need to be called, they are called in an order the same as the order of the hooks applied to the component. Finally, the last argument passes any extra information required by the hook. After handling the event, we see that the same `InvokeHook` function is called, only changing the third argument to `AfterEventHookPos`.

`BeforeEventHookPos` and `AfterEventHookPos` are not all the possible hook positions and users can customize the positions that accept hooks. For example, in GCN3 model, we need to provide hook positions for the instruction pipelines, so that we can monitor the progress of the instruction execution. We can define a hook position in the following way: 

```go
var InstReadStartHookPos = &struct{ name string }{"InstReadStart"}
``` 

This position variable is actually a global variable (do no panic, it is not bad here). The part `&struct{ name string }` declares an anoymous struct type with only 1 string field called. We immediately instantiate it with `{"InstReadStart"}`, setting the name to `InstReadStart`.

## Implementing Hooks

Implementing a hook needs to implement 3 functions. 

With the `Type` function, we specify what type triggers the hook. This function returns a variable of type `reflect.Type`. Here is an example of how to implement the function:

```go
func (h *EventLogger) Type() reflect.Type {
    return reflect.TypeOf((Event)(nil))
}
```

In this example, we see that we first cast a nil pointer to type Event then we use `reflect.TypeOf` to get the type. Since this function returns type `Event`, this hook can be triggered by all types of events.

The `Pos` function is straightforward, and it simply needs to return the position that the hook is called. An example of the `Pos` function is: 

```go
func (h *EventLogger) Pos() HookPos {
    return BeforeEventHookPos
}

```

Finally, the `Func` function defines what does the hook do. Here, we use the simplified `EventLogger` as an example again and see how it logs the events. 

```go
func (h *EventLogger) Func(
    item interface{},
    domain Hookable,
    info interface{},
) {
    evt := item.(Event)
    h.Logger.Printf("%.10f, %s", evt.Time(), reflect.TypeOf(evt))
}
```


## Predefined Hook

Project Akita and each simulator model provide predefined hooks. In project Akita, we provide the `EventLogger` hook. It can be applied to any Component or any event-driven simulation engine. If it is hooked to an event-driven simulation engine, it logs all the events triggered. If it hooks to a Component, it logs the events handled by the component.
