package simulation

import (
	"github.com/rs/xid"
	"github.com/sarchlab/akita/v4/datarecording"
	"github.com/sarchlab/akita/v4/monitoring"
	"github.com/sarchlab/akita/v4/sim"
	"github.com/sarchlab/akita/v4/tracing"
)

// A Simulation provides the service requires to define a simulation.
type Simulation struct {
	engine sim.Engine

	dataRecorder datarecording.DataRecorder
	monitor      *monitoring.Monitor
	visTracer    *tracing.DBTracer

	components    []sim.Component
	compNameIndex map[string]int
	ports         []sim.Port
	portNameIndex map[string]int
}

// NewSimulation creates a new simulation.
func NewSimulation() *Simulation {
	s := &Simulation{
		compNameIndex: make(map[string]int),
		portNameIndex: make(map[string]int),
	}

	name := xid.New().String()
	s.dataRecorder = datarecording.NewDataRecorder(
		"akita_sim_" + name + ".sqlite3")

	s.monitor = monitoring.NewMonitor()
	s.visTracer = tracing.NewDBTracer(s.engine, s.dataRecorder)

	return s
}

// RegisterEngine registers the engine used in the simulation.
func (s *Simulation) RegisterEngine(e sim.Engine) {
	s.engine = e
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

// RegisterComponent registers a component with the simulation.
func (s *Simulation) RegisterComponent(c sim.Component) {
	compName := c.Name()
	if s.compNameIndex[compName] != 0 {
		panic("component " + compName + " already registered")
	}

	s.components = append(s.components, c)
	s.compNameIndex[compName] = len(s.components) - 1

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
}
