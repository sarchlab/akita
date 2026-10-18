package sim

// Monitor observes a simulation without tying the runtime to a user interface.
// Successful Initialize calls Start once after component state and topology are
// complete. The monitor discovers objects through Components and Ports.
// Terminate calls Stop only if Start completed, before closing the tracer and
// recording. Each monitor belongs to one simulation and must not be reused.
type Monitor interface {
	Start(*Simulation)
	Stop()
}
