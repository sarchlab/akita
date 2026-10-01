// Package wakeup defines wakeup components: components without a clock that
// run when they are woken.
//
// # When to use a wakeup component
//
// Choose a wakeup component when the component is idle most of the time and
// knows when it next has work, like a unit whose latency is computed rather
// than counted cycle by cycle. It runs only when woken, so an idle component
// costs nothing, but it must ask for every future wakeup itself; a wakeup it
// forgets to ask for stalls it. Choose modeling/ticking when the work
// advances cycle by cycle, and modeling/event when each happening carries
// its own data.
//
// # How it runs
//
// The component is woken when a port receives a message or frees buffer
// space, or at a time a middleware asked for with WakeAt. Each wakeup is an
// Event, which carries no data: every middleware, in field order, looks at the
// State and ports and does the work that is ready. If
// any middleware made progress, the component wakes again at the same time,
// so it keeps running until no middleware has work ready. A middleware whose
// work becomes ready later calls WakeAt for that time.
//
// A middleware implements modeling.Middleware:
//
//	func (m *sendMW) Handle(_ timing.Event) bool {
//	    now := m.comp.CurrentTime()
//	    for _, p := range m.comp.State.Pending {
//	        if p.At > now {
//	            m.comp.WakeAt(p.At) // not ready yet
//	            continue
//	        }
//	        ... // send it
//	    }
//	    ...
//	}
//
// # Five structs
//
// A wakeup component type is defined by five structs, all declared in the
// component's package, exactly as for a ticking component except that Spec
// needs no Freq. The system builder, the code that assembles a simulation,
// supplies the Spec, the Resources, and the Ports; the component creates its
// State with NewState and its middlewares with NewMiddlewares. Middlewares
// hold only references; all mutable data lives in State, because only State
// is saved in checkpoints.
//
// # Declaring and building a component
//
//	type Comp = wakeup.Component[Spec, State, Resources, Ports, Middlewares]
//
//	var Definition = wakeup.Definition[Spec, State, Resources, Ports, Middlewares]{
//	    DefaultSpec:    Spec{Latency: 100},
//	    NewState:       newState, // optional
//	    NewMiddlewares: newMiddlewares,
//	}
//
//	comp := agent.Definition.Builder().
//	    WithSimulation(sim).
//	    WithSpec(spec).
//	    WithPorts(agent.Ports{Out: messaging.NewPort(nil, 4, 4, "Agent.Out")}).
//	    Build("Agent")
//
// Every port is created by the system builder and named "<instance>.<field>",
// or "<instance>.<field>[i]" for member i of a port group; Build binds and
// registers them, and no port is added later. A component type is identified
// by its package (TypeName); Name returns the instance name given to Build.
package wakeup
