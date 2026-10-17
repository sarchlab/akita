// Package monitoring provides Akita's live monitoring web server. It implements
// sim.Monitor and exposes component inspection, progress bars, tracing
// controls, and profiling without coupling the simulation runtime to a UI.
//
// Usage:
//
//	monitor := monitoring.MakeBuilder().WithPortNumber(8080).Build()
//	s := sim.MakeBuilder().WithMonitor(monitor).Build()
//	defer s.Terminate()
package monitoring
