// Package ticking defines ticking components: components that run on a
// clock.
//
// # When to use a ticking component
//
// Choose a ticking component when the behavior naturally advances cycle by
// cycle, like a pipeline or a cache. On every tick, the component's
// middlewares look at its State and ports and do one cycle of work. The
// component sleeps when no middleware makes progress and wakes when a port
// receives a message or frees buffer space. It is the default choice; choose
// modeling/wakeup for a component that is idle most of the time and knows when
// it next has work, and modeling/event for one whose behavior is a set of
// reactions to events that carry data.
//
// # Middlewares handle events
//
// A middleware implements modeling.Middleware: Handle(e timing.Event) bool.
// The component passes every event it receives to all of its middlewares in
// field order. A tick is a modeling.TickEvent, and most middlewares do their
// cycle of work without looking at the event. A component may also schedule
// events for itself; they reach the middlewares the same way, and a
// middleware ignores the events it does not handle. Handle returns true if
// the middleware made progress, and the component ticks again on the next
// cycle if any middleware did.
//
// # Five structs
//
// A ticking component type is defined by five structs, all declared in the
// component's package. The component defines every type; what differs is who
// supplies the values. The system builder is the code that assembles a
// simulation: it builds each component instance, gives it a configuration,
// and wires it to the rest of the system.
//
//   - Spec is the configuration: scalar fields only. The component gives the
//     defaults in its Definition, and the system builder may override them.
//     Spec must have a Freq field of type timing.Freq, the clock the
//     component ticks at.
//   - State is the mutable runtime data. The component creates it with the
//     Definition's NewState, or starts from the zero State, and only the
//     component's middlewares change it. It is saved in checkpoints.
//   - Resources holds references to objects shared with other components,
//     such as a backing storage. The system builder supplies them.
//   - Ports has one messaging.Port field per port and one []messaging.Port
//     field per port group. The system builder creates the port instances,
//     choosing their buffer sizes, and passes them to Build.
//   - Middlewares has one field per middleware. The component creates them
//     with the Definition's NewMiddlewares, and they handle every event in
//     field order. Middlewares hold only references; all mutable data lives
//     in State, because only State is saved in checkpoints.
//
// # Declaring and building a component
//
// The component package declares its Definition and a Comp alias:
//
//	type Comp = ticking.Component[Spec, State, Resources, Ports, Middlewares]
//
//	var Definition = ticking.Definition[Spec, State, Resources, Ports, Middlewares]{
//	    DefaultSpec:    Spec{Freq: 1 * timing.GHz},
//	    NewState:       newState, // optional
//	    NewMiddlewares: newMiddlewares,
//	}
//
// The system builder builds an instance from it:
//
//	comp := cache.Definition.Builder().
//	    WithSimulation(sim).
//	    WithSpec(spec).
//	    WithResources(res).
//	    WithPorts(cache.Ports{Top: top, Bottom: bottom}).
//	    Build("GPU[0].L1Cache")
//
// The system builder creates every port with messaging.NewPort, choosing its
// buffer sizes, and names it "<instance>.<field>", for example
// "GPU[0].L1Cache.Top", or "<instance>.<field>[i]" for member i of a port
// group. Build binds each port to the instance and registers it with the
// simulation; no port is added after Build. Middlewares reach the ports
// through the Ports fields.
//
// # Type name and instance name
//
// A component type is identified by its package: TypeName returns the
// package's import path, and each package declares at most one component.
// Name returns the instance name given to Build.
package ticking
