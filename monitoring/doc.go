// Package monitoring provides Akita's live monitoring web server. It implements
// sim.Monitor and exposes component inspection, progress bars, tracing
// controls, and profiling without coupling the simulation runtime to a UI.
//
// Usage:
//
//	monitor := monitoring.MakeBuilder().WithPortNumber(8080).Build()
//	s := sim.MakeBuilder().WithMonitor(monitor).Build()
//	defer s.Terminate()
//	// Assemble components and bind ports, then start monitoring:
//	if err := s.Initialize(); err != nil { panic(err) }
package monitoring
