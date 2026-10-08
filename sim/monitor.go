package sim

import "github.com/sarchlab/akita/v5/sim/naming"

// Monitor observes a simulation without tying the runtime to a user interface.
// Build calls Start once after creating runtime services, before application
// components are assembled and Initialize is called. It forwards registrations as those
// objects are built. Terminate calls Stop before closing the tracer and data
// recorder. Each monitor belongs to one simulation and must not be reused.
type Monitor interface {
	Start(s *Simulation)
	RegisterComponent(c naming.Named)
	RegisterPort(p Port)
	Stop()
}
