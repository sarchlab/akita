// Package event defines event components: components without a clock whose
// behavior is a set of reactions to events that carry data.
//
// # When to use an event component
//
// Choose an event component when the behavior is a set of distinct
// happenings, each with its own data and time: a request that completes after
// a computed latency, a timer that fires with a payload, a traffic
// generator's timed actions. Each middleware reacts to the events it cares
// about instead of polling the State. Choose modeling/ticking when the work
// advances cycle by cycle, and modeling/wakeup when the component polls its
// State and ports whenever it runs.
//
// # How it runs
//
// Everything that happens to an event component is an event, passed to
// every middleware in field order:
//
//   - Recv, when a port receives a message into an empty incoming buffer;
//   - PortFree, when a full outgoing buffer gains a slot or the connection
//     signals that it can take messages again;
//   - the events the component schedules for itself with Schedule.
//
// A middleware checks the event's type and ignores the events it does not
// handle. The component runs only when an event arrives; the progress a
// middleware reports is not used.
//
//	type doneEvent struct {
//	    timing.EventBase
//	    ReqID uint64 `json:"req_id"`
//	}
//
//	func init() { timing.RegisterEvent(doneEvent{}) }
//
//	func (m *memMW) Handle(e timing.Event) bool {
//	    switch e := e.(type) {
//	    case event.Recv:
//	        ... // take each request, then
//	        m.comp.Schedule(doneEvent{
//	            EventBase: m.comp.MakeEventBase(e.Time() + latency),
//	            ReqID:     req.ID,
//	        })
//	    case doneEvent:
//	        ... // respond to e.ReqID
//	    default:
//	        return false
//	    }
//	    return true
//	}
//
// Register each event type the component schedules with timing.RegisterEvent,
// so a checkpoint can hold it while it is pending; a forgotten registration
// fails when the checkpoint is loaded. A Recv is not repeated until the port's
// incoming buffer empties, so a middleware takes every message it can and
// schedules a retry for the rest; likewise it keeps a response it cannot send
// in the State and sends it on PortFree.
//
// # Five structs
//
// An event component type is defined by five structs, all declared in the
// component's package, exactly as for a ticking component except that Spec
// needs no Freq. The system builder, the code that assembles a simulation,
// supplies the Spec, the Resources, and the Ports; the component creates its
// State with NewState and its middlewares with NewMiddlewares. Middlewares
// hold only references; all mutable data lives in State or in the pending
// events, because only those are saved in checkpoints.
//
// # Declaring and building a component
//
//	type Comp = event.Component[Spec, State, Resources, Ports, Middlewares]
//
//	var Definition = event.Definition[Spec, State, Resources, Ports, Middlewares]{
//	    DefaultSpec:    Spec{Latency: 100},
//	    NewState:       newState, // optional
//	    NewMiddlewares: newMiddlewares,
//	}
//
//	comp := mem.Definition.Builder().
//	    WithSimulation(sim).
//	    WithSpec(spec).
//	    Build("Mem")
//
// After Build, bind each declared slot with comp.BindPort("Top", port).
// The component assigns the owner and full name, such as "GPU[0].L1Cache.Top".
// Use "Links[0]" for an indexed port group. Bind connections with
// conn.BindPort(port), then call the shared simulation's Initialize method.
// Initialize validates all ports, freezes topology, and creates State and
// Middlewares. Only then seed work and run the engine. Ports and Middlewares
// must not be reassigned directly.
package event
