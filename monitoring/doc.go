// Package monitoring provides Akita's live monitoring web server. It implements
// sim.Monitor and exposes component inspection, progress bars, tracing
// controls, and profiling without coupling the simulation runtime to a UI.
//
// Usage:
//
//	s := sim.MakeBuilder().
//	    WithMonitorFactory(func(s *sim.Simulation) sim.Monitor {
//	        return monitoring.NewMonitor(s).WithPortNumber(8080)
//	    }).Build()
//	defer s.Terminate()
package monitoring
