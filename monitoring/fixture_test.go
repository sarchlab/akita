package monitoring

import "github.com/sarchlab/akita/v5/sim/timing"

// Handler unit tests supply only the services exercised by that handler.
func newTestMonitorWithSimulation(s timing.Simulation) *Monitor {
	m := NewMonitor()
	m.simulation = s
	m.engine = s.Engine()
	return m
}
