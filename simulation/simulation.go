package simulation

import (
	"github.com/sarchlab/akita/v4/datarecording"
	"github.com/sarchlab/akita/v4/monitoring"
	"github.com/sarchlab/akita/v4/sim"
	"github.com/sarchlab/akita/v4/tracing"
)

// A Simulation provides the service requires to define a simulation.
type Simulation struct {
	id           string
	engine       sim.Engine
	dataRecorder datarecording.DataRecorder
	monitor      *monitoring.Monitor
	visTracer    *tracing.DBTracer

	components    []sim.Component
	compNameIndex map[string]int
	ports         []sim.Port
	portNameIndex map[string]int
}

// ID returns the ID of the simulation. An ID is a UUID that is generated when
// the simulation is created.
func (s *Simulation) ID() string {
	return s.id
}

// GetEngine returns the engine used in the simulation.
func (s *Simulation) GetEngine() sim.Engine {
	return s.engine
}

// GetDataRecorder returns the data recorder used in the simulation.
func (s *Simulation) GetDataRecorder() datarecording.DataRecorder {
	return s.dataRecorder
}

// GetMonitor returns the monitor used in the simulation.
func (s *Simulation) GetMonitor() *monitoring.Monitor {
	return s.monitor
}

// GetVisTracer returns the tracer used in the simulation.
func (s *Simulation) GetVisTracer() *tracing.DBTracer {
	return s.visTracer
}

// Components returns all the components registered in the simulation. The
// returned slice should be treated as read-only.
func (s *Simulation) Components() []sim.Component {
	return s.components
}

// RegisterComponent registers a component with the simulation.
func (s *Simulation) RegisterComponent(c sim.Component) {
	compName := c.Name()
	if s.compNameIndex[compName] != 0 {
		panic("component " + compName + " already registered")
	}

	s.components = append(s.components, c)
	s.compNameIndex[compName] = len(s.components) - 1

	if s.monitor != nil {
		s.monitor.RegisterComponent(c)
	}

	for _, p := range c.Ports() {
		s.registerPort(p)
	}
}

// registerPort registers a port with the simulation.
func (s *Simulation) registerPort(p sim.Port) {
	portName := p.Name()
	if s.portNameIndex[portName] != 0 {
		panic("port " + portName + " already registered")
	}

	s.ports = append(s.ports, p)
	s.portNameIndex[portName] = len(s.ports) - 1
}

// GetComponentByName returns the component with the given name.
func (s *Simulation) GetComponentByName(name string) sim.Component {
	return s.components[s.compNameIndex[name]]
}

// GetPortByName returns the port with the given name.
func (s *Simulation) GetPortByName(name string) sim.Port {
	return s.ports[s.portNameIndex[name]]
}

// Terminate terminates the simulation.
func (s *Simulation) Terminate() {
	s.dataRecorder.Close()

	if s.monitor != nil {
		s.monitor.StopServer()
	}
}
