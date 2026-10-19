package monitoring

import (
	"github.com/sarchlab/akita/v5/internal/testsim"
)

// Handler unit tests supply only the services exercised by that handler.
func newTestMonitorWithSimulation(s *testsim.Simulation) *Monitor {
	m := MakeBuilder().Build()
	m.simulation = s
	m.engine = s.Engine()
	m.runSimulation = s.Engine().Run
	return m
}

func (m *Monitor) addTestComponent(c Component) { m.components = append(m.components, c) }
