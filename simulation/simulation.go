package simulation

import (
	"github.com/sarchlab/akita/v5/daisen2"
	"github.com/sarchlab/akita/v5/datarecording"

	"github.com/sarchlab/akita/v5/monitoring2"
	"github.com/sarchlab/akita/v5/timing"
	"github.com/sarchlab/akita/v5/tracing"
)

type Simulation struct {
	id           string
	outputPath   string
	engine       timing.Engine
	dataRecorder datarecording.DataRecorder
	visTracer    *tracing.DBTracer
	metaRecorder *metaRecorder
	monitor      *monitoring2.Monitor

	components        []Component
	compNameIndex     map[string]int
	ports             []Port
	portNameIndex     map[string]int
	connections       []Connection
	connNameIndex     map[string]int
	resources         []Resource
	resourceNameIndex map[string]int

	entities        []Entity
	entityNameIndex map[EntityKind]map[string]int
}

// ID returns the ID of the simulation. An ID is a UUID that is generated when
// the simulation is created.
func (s *Simulation) ID() string {
	return s.id
}

// GetEngine returns the engine used in the simulation.
func (s *Simulation) GetEngine() timing.Engine {
	return s.engine
}

// GetDataRecorder returns the data recorder used in the simulation.
func (s *Simulation) GetDataRecorder() datarecording.DataRecorder {
	return s.dataRecorder
}

// GetMonitor returns the live monitor attached to the simulation, if enabled.
func (s *Simulation) GetMonitor() *monitoring2.Monitor {
	return s.monitor
}

// GetServer is kept for compatibility with older monitoring setup code.
// Monitoring2 does not expose a Daisen2 replay server.
func (s *Simulation) GetServer() *daisen2.Server {
	if s.monitor == nil {
		return nil
	}

	return s.monitor.GetServer()
}

// GetVisTracer returns the tracer used in the simulation.
func (s *Simulation) GetVisTracer() *tracing.DBTracer {
	return s.visTracer
}

// Components returns all the components registered in the simulation. The
// returned slice should be treated as read-only.
func (s *Simulation) Components() []Component {
	return s.components
}

// RegisterComponent registers a component with the simulation.
func (s *Simulation) RegisterComponent(c Component) {
	compName := c.Name()
	if _, found := s.compNameIndex[compName]; found {
		panic("component " + compName + " already registered")
	}
	s.registerEntity(componentEntity(compName))

	s.components = append(s.components, c)
	s.compNameIndex[compName] = len(s.components) - 1

	if hookable, ok := c.(tracing.NamedHookable); ok {
		tracing.CollectTrace(hookable, s.visTracer)
	}

	if s.monitor != nil {
		s.monitor.RegisterComponent(c)
	}

	for _, p := range componentPorts(c) {
		s.registerPort(p)
	}

	if owner, ok := c.(ResourceOwner); ok {
		for _, resource := range owner.Resources() {
			s.registerResource(resource)
		}
	}
}

// registerPort registers a port with the simulation.
func (s *Simulation) registerPort(p Port) {
	portName := p.Name()
	if _, found := s.portNameIndex[portName]; found {
		panic("port " + portName + " already registered")
	}
	s.registerEntity(portEntity(portName))

	s.ports = append(s.ports, p)
	s.portNameIndex[portName] = len(s.ports) - 1
}

// RegisterConnection registers a connection with the simulation runtime
// inventory. Setup code still owns topology construction and PlugIn calls, but
// registered connections can be validated and checkpointed as runtime owners.
func (s *Simulation) RegisterConnection(c Connection) {
	connName := c.Name()
	if _, found := s.connNameIndex[connName]; found {
		panic("connection " + connName + " already registered")
	}
	s.registerEntity(connectionEntity(connName))

	s.connections = append(s.connections, c)
	s.connNameIndex[connName] = len(s.connections) - 1
}

// Connections returns all registered connections. The returned slice should be
// treated as read-only.
func (s *Simulation) Connections() []Connection {
	return s.connections
}

// RegisterResource registers non-timing program state that can be referenced
// by multiple components and checkpointed independently.
func (s *Simulation) RegisterResource(r Resource) {
	s.registerResource(r)
}

func (s *Simulation) registerResource(r Resource) {
	if r == nil {
		panic("resource cannot be nil")
	}

	name := r.Name()
	if name == "" {
		panic("resource name cannot be empty")
	}

	identity := r.Identity()
	if identity == "" {
		panic("resource " + name + " identity cannot be empty")
	}

	if idx, found := s.resourceNameIndex[name]; found {
		existing := s.resources[idx]
		if existing.Identity() != identity {
			panic("resource " + name + " already registered")
		}

		return
	}
	s.registerEntity(resourceEntity(r))

	s.resources = append(s.resources, r)
	s.resourceNameIndex[name] = len(s.resources) - 1
}

// Resources returns all shared-state resources registered in the simulation.
// The returned slice should be treated as read-only.
func (s *Simulation) Resources() []Resource {
	return s.resources
}

// Entities returns a stable snapshot of all registered simulation entities.
func (s *Simulation) Entities() []Entity {
	return append([]Entity(nil), s.entities...)
}

// GetComponentByName returns the component with the given name.
func (s *Simulation) GetComponentByName(name string) Component {
	idx, found := s.compNameIndex[name]
	if !found {
		panic("component " + name + " not registered")
	}

	return s.components[idx]
}

// GetPortByName returns the port with the given name.
func (s *Simulation) GetPortByName(name string) Port {
	idx, found := s.portNameIndex[name]
	if !found {
		panic("port " + name + " not registered")
	}

	return s.ports[idx]
}

// Terminate terminates the simulation.
func (s *Simulation) Terminate() {
	if s.monitor != nil {
		s.monitor.StopServer()
	}

	if s.visTracer != nil {
		s.visTracer.Terminate()
	}

	if s.metaRecorder != nil {
		s.metaRecorder.End()
	}

	s.dataRecorder.Close()
}
