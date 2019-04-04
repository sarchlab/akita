# The Event System

Akita framework is pure event-driven.

An `Event` is defined as a state update at a certain time. An `Event` only has 2 standard fields, the `Time` and the `Handler` of the event. A certain event can be usually handled by different handlers and may show different behaviors.

An event-driven simulation engine, or `Engine` for short, maintains all the events in the simulator. It defines two core functions `Schedule` and `Run`. When a simulation starts, at least one initial `Event` needs to be scheduled. When a `Handler` handles an `Event`, the handler can schedule other Events to happen in the future.  The `Run` function triggers all the events in the chronological order until no more `Event` left in the event queue.

By default, 2 different types of `Engine` are provided. It is recommended to use the `SerialEngine` if a large number of simulations need to run. Otherwise, if the user wants to get the result for a single simulation as fast as possible, the `ParallelEngine` should be used. Note that the `SerialEngine` does not only use one CPU core, as the Go runtime and libraries may use multiple cores.