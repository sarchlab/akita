// Package monitoring provides Akita's live monitoring web server. It implements
// simulation.Monitor and exposes component inspection, progress bars, tracing
// controls, and profiling without coupling the simulation runtime to a UI.
//
// Usage:
//
//	monitor := monitoring.NewMonitor().WithPortNumber(8080)
//	sim := simulation.MakeBuilder().WithMonitor(monitor).Build()
//	defer sim.Terminate()
package monitoring
