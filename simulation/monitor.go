package simulation

import (
	"github.com/sarchlab/akita/v5/simulation/naming"
	"github.com/sarchlab/akita/v5/simulation/timing"
	"github.com/sarchlab/akita/v5/simulation/tracing"
)

// Monitor observes a simulation without tying the runtime to a user interface.
// Build calls Start once, before any application components or ports are
// registered. Registrations are forwarded as those objects are built.
// Terminate calls Stop before closing the tracer and data recorder.
//
// Each monitor instance belongs to a single simulation. Implementations may
// use the runtime services, tracer, and SQLite recording path supplied to
// Start, but the simulation owns and closes those resources.
type Monitor interface {
	Start(sim timing.Simulation, tracer *tracing.DBTracer, traceDBPath string)
	RegisterComponent(c naming.Named)
	RegisterPort(p Port)
	Stop()
}
