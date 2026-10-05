package monitoring

import (
	"github.com/sarchlab/akita/v5/monitoring/static"
	"github.com/sarchlab/akita/v5/sim/timing"
)

// Handler unit tests supply only the services exercised by that handler.
func newTestMonitor() *Monitor {
	return &Monitor{fs: static.GetAssets()}
}

func newTestMonitorWithSimulation(s timing.Simulation) *Monitor {
	m := newTestMonitor()
	m.simulation = s
	m.engine = s.Engine()
	return m
}
