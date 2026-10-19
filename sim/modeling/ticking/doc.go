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
// A middleware implements ticking.Middleware: Handle(e timing.Event) bool.
// The component passes every event it receives to all of its middlewares in
// field order. Every event a ticking component receives is an Event, so
// most middlewares do their cycle of work without looking at the event.
// Handle returns true if the middleware made progress, and the component
// ticks again on the next cycle if any middleware did. A ticking component
// does not schedule events of its own: a component that needs events with
// data at times it chooses is written in the event model (modeling/event).
//
// # Five structs
//
// A ticking component type is defined by five structs, all declared in the
// component's package. The component defines every type; what differs is who
// supplies the values. The system builder is the code that assembles a
// simulation: it builds each component instance, gives it a configuration,
// and wires it to the rest of the system.
//
//   - Spec is the configuration: scalars, scalar collections, flat records, and
//     record slices. See modeling.ValidateSpec for the allowed record members.
//     The component gives defaults in DefaultSpec(), and the system builder may
//     override them.
//     Spec must have a Freq field of type timing.Freq, the clock the
//     component ticks at.
//   - State is the mutable runtime data. The component creates it with the
//     State.Initialize, or starts from the zero State, and only the
//     component's middlewares change it. It is saved in checkpoints.
//   - Resources holds references to objects shared with other components,
//     such as a backing storage. The system builder supplies them.
//   - Ports has one messaging.Port field per port and one []messaging.Port
//     field per port group. The system builder creates the port instances,
//     choosing their buffer sizes, and binds them after Build.
//   - Middlewares has one field per middleware. The component creates them
//     with the Middlewares.Initialize, and they handle every event in
//     field order. Middlewares hold only references; all mutable data lives
//     in State, because only State is saved in checkpoints.
//
// # Declaring and building a component
//
// The component package declares its DefaultSpec function and a Comp alias:
//
//	type Comp = ticking.Component[Spec, State, Resources, Ports, Middlewares]
//
//	func DefaultSpec() Spec {
//	    return Spec{Freq: 1 * timing.GHz}
//	}
//
// The system builder builds an instance from the returned configuration:
//
//	comp := modeling.MakeBuilder[*cache.Comp](cache.DefaultSpec()).
//	    WithSimulation(sim).
//	    WithSpec(spec).
//	    WithResources(res).
//	    Build("GPU[0].L1Cache")
//
// After Build, bind each declared slot with comp.BindPort("Top", port).
// The component assigns the owner and full name, such as "GPU[0].L1Cache.Top".
// Use "Links[0]" for an indexed port group. Bind connections with
// conn.BindPort(port), then call the shared simulation's Initialize method.
// Initialize validates all ports, freezes topology, and creates State and
// Middlewares. Only then seed work and run the engine. Ports and Middlewares
// must not be reassigned directly.
//
// # Type name and instance name
//
// A component type is identified by its package's import path, and each
// package declares at most one component. Name returns the instance name
// given to Build.
package ticking
