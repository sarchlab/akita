package simulation

import (
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

	components    []Component
	compNameIndex map[string]int
	ports         []Port
	portNameIndex map[string]int
	connections   []Connection
	connNameIndex map[string]int
	resources     []Resource

	// entities is the single, flat inventory of every registered runtime object
	// (components, ports, connections, and resources), each held as the Entity
	// it satisfies. entityByName resolves a globally unique name to its index.
	// Together they back the global state manager and GetStateByName. The engine
	// and ID generator are not entities: the engine is the engine field (see
	// GetEngine) and the ID generator is a timing-package global.
	entities     []Entity
	entityByName map[string]int
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

// GetVisTracer returns the tracer used in the simulation.
func (s *Simulation) GetVisTracer() *tracing.DBTracer {
	return s.visTracer
}

// Components returns a copy of the registered components, in registration
// order.
func (s *Simulation) Components() []Component {
	return append([]Component(nil), s.components...)
}

// registerEntity records a live entity in the single, flat inventory. It is the
// only cross-kind uniqueness check — names must be globally unique across all
// kinds, which is what makes GetStateByName well defined. The typed Register
// methods pass the concrete object, which satisfies Entity, so the inventory
// holds the live entity itself.
func (s *Simulation) registerEntity(e Entity) {
	name := e.Name()
	if name == "" {
		panic("entity name cannot be empty")
	}

	if s.entityByName == nil {
		s.entityByName = make(map[string]int)
	}

	if _, found := s.entityByName[name]; found {
		panic("entity " + name + " already registered")
	}

	s.entities = append(s.entities, e)
	s.entityByName[name] = len(s.entities) - 1
}

// RegisterComponent registers a component with the simulation.
func (s *Simulation) RegisterComponent(c Component) {
	compName := c.Name()
	s.registerEntity(c)

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
}

// registerPort registers a port with the simulation.
func (s *Simulation) registerPort(p Port) {
	portName := p.Name()
	s.registerEntity(p)

	s.ports = append(s.ports, p)
	s.portNameIndex[portName] = len(s.ports) - 1
}

// RegisterConnection registers a connection with the simulation runtime
// inventory. Setup code still owns topology construction and PlugIn calls, but
// registered connections are tracked as runtime entities in the global state
// manager.
func (s *Simulation) RegisterConnection(c Connection) {
	connName := c.Name()
	s.registerEntity(c)

	s.connections = append(s.connections, c)
	s.connNameIndex[connName] = len(s.connections) - 1
}

// Connections returns all registered connections. The returned slice should be
// treated as read-only.
func (s *Simulation) Connections() []Connection {
	return s.connections
}

// RegisterResource registers non-timing program state that can be referenced by
// multiple components and reached by name through the global state manager. The
// simulation owns the resource; components hold references to it. Setup
// constructs and registers each shared resource once under a canonical name.
func (s *Simulation) RegisterResource(r Resource) {
	if r == nil {
		panic("resource cannot be nil")
	}

	s.registerEntity(r)
	s.resources = append(s.resources, r)
}

// Resources returns all shared-state resources registered in the simulation.
// The returned slice should be treated as read-only.
func (s *Simulation) Resources() []Resource {
	return s.resources
}

// Entities returns a stable snapshot of all registered simulation entities, in
// registration order.
func (s *Simulation) Entities() []Entity {
	return append([]Entity(nil), s.entities...)
}

// GetStateByName resolves a registered entity name to the live entity. It is the
// global state-access backdoor: a component can reach designed shared state
// (such as a page table or memory resource) by name and mutate it in place.
// Resolve the reference once at setup and cache it; this is a map lookup, not a
// free dereference.
//
// The returned value is the registered entity itself (a component, port,
// connection, resource, etc.); callers type-assert it to the concrete type.
// That friction is intentional — it flags that you are reaching past the normal
// interfaces to another entity.
func (s *Simulation) GetStateByName(name string) (State, bool) {
	idx, found := s.entityByName[name]
	if !found {
		return nil, false
	}

	return s.entities[idx], true
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
