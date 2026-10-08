package sim

import (
	"github.com/sarchlab/akita/v5/internal/assembly"
	"github.com/sarchlab/akita/v5/sim/datarecording"
	"github.com/sarchlab/akita/v5/sim/hooking"
	"github.com/sarchlab/akita/v5/sim/naming"
	"github.com/sarchlab/akita/v5/sim/timing"
	"github.com/sarchlab/akita/v5/sim/tracing"
)

type Simulation struct {
	setup            assembly.Setup
	id               string
	outputPath       string
	engine           timing.Engine
	idGenerator      *timing.IDGenerator
	dataRecorder     datarecording.DataRecorder
	visTracer        *tracing.DBTracer
	metaRecorder     *metaRecorder
	topologyRecorder *topologyRecorder
	monitor          Monitor

	// components and ports are kept in registration order for the topology
	// recorder.
	components []Component
	ports      []Port

	// entities is the single, flat inventory of every registered runtime object
	// (components, ports, connections, resources, the engine, and the ID
	// generator), each held as the Entity it satisfies. entityByName resolves a
	// globally unique name to its index. Together they make the inventory a
	// complete state snapshot: serializing every entity's state captures
	// everything needed to recover the simulation. The engine is additionally
	// held in the engine field for direct typed access (see Engine).
	entities     []Entity
	entityByName map[string]int
}

// ID returns the ID of the simulation. An ID is a UUID that is generated when
// the simulation is created.
func (s *Simulation) ID() string {
	return s.id
}

// Engine returns the engine used in the simulation.
func (s *Simulation) Engine() timing.Engine {
	return s.engine
}

// DataRecorder returns the data recorder used in the simulation. A simulator
// can write its own tables into the same recording.
func (s *Simulation) DataRecorder() datarecording.DataRecorder {
	return s.dataRecorder
}

// VisTracer returns the visualization tracer owned by the simulation.
// The tracer exists even when monitoring and tracing are disabled.
func (s *Simulation) VisTracer() *tracing.DBTracer {
	return s.visTracer
}

// TraceDBPath returns the SQLite recording path, including the file extension.
func (s *Simulation) TraceDBPath() string {
	return s.outputPath + ".sqlite3"
}

// Monitor returns the live monitor attached to the simulation, if enabled.
func (s *Simulation) Monitor() Monitor {
	return s.monitor
}

// registerEntity records a live entity in the single, flat inventory. It is the
// only cross-kind uniqueness check — names must be globally unique across all
// kinds, which is what makes the inventory a well-defined state snapshot. The
// typed Register methods pass the concrete object, which satisfies Entity, so
// the inventory holds the live entity itself.
func (s *Simulation) registerEntity(e Entity) {
	s.setup.Register(e)
	name := e.Name()

	if s.entityByName == nil {
		s.entityByName = make(map[string]int)
	}

	s.entities = append(s.entities, e)
	s.entityByName[name] = len(s.entities) - 1
}

// RegisterComponent registers a component with the simulation. It accepts any
// named object so that component builders can register through the
// timing.Simulation interface without importing this package.
func (s *Simulation) RegisterComponent(c naming.Named) {
	s.registerEntity(c)
	s.components = append(s.components, c)

	if hookable, ok := c.(interface {
		naming.Named
		hooking.Hookable
	}); ok {
		tracing.CollectTrace(hookable, s.visTracer)
	}

	if s.monitor != nil {
		s.monitor.RegisterComponent(c)
	}
}

// RegisterPort registers a port with the simulation so it can be resolved by
// name and monitored. A component model's BindPort registers each port it binds;
// an owner written without a component model registers its own ports.
func (s *Simulation) RegisterPort(p naming.Named) {
	port, ok := p.(Port)
	if !ok {
		panic("simulation: RegisterPort requires a messaging.Port, got " +
			p.Name())
	}

	s.registerEntity(port)
	s.ports = append(s.ports, port)

	// Attach incoming- and outgoing-buffer tracing, mirroring how
	// RegisterComponent attaches CollectTrace. The resulting tasks flow to the
	// component tracer, so they only materialize for components that are
	// themselves traced.
	tracing.CollectIncomingBufferTrace(p)
	tracing.CollectOutgoingBufferTrace(p)

	if s.monitor != nil {
		s.monitor.RegisterPort(port)
	}
}

// RegisterConnection registers a connection with the simulation runtime
// inventory. Setup code still owns topology construction and BindPort calls, but
// registered connections are tracked as runtime entities in the global state
// manager.
func (s *Simulation) RegisterConnection(c naming.Named) {
	s.registerEntity(c)
}

// RegisterResource registers non-timing program state that can be referenced by
// multiple components and reached by name through the global state manager. The
// simulation owns the resource; components hold references to it. Setup
// constructs and registers each shared resource once under a canonical name.
func (s *Simulation) RegisterResource(r naming.Named) {
	if r == nil {
		panic("resource cannot be nil")
	}

	s.registerEntity(r)
}

// Terminate terminates the simulation.
func (s *Simulation) Terminate() {
	if s.monitor != nil {
		s.monitor.Stop()
	}

	if s.visTracer != nil {
		s.visTracer.Terminate()
	}

	if s.metaRecorder != nil {
		s.metaRecorder.End()
	}

	if s.topologyRecorder != nil {
		s.topologyRecorder.Record(s.components, s.ports)
	}

	s.dataRecorder.Close()
}

// NewID allocates an ID unique within this simulation.
func (s *Simulation) NewID() uint64 { return s.idGenerator.NewID() }

// Initialize validates the completed wiring, freezes topology, and creates state
// and middleware. Call it once before seeding work or loading a checkpoint.
func (s *Simulation) Initialize() error { return s.setup.Initialize() }

// RequireSetup rejects structural changes after initialization begins.
func (s *Simulation) RequireSetup() { s.setup.RequireSetup() }

// RequireNameAvailable validates a name before a binding changes either side.
func (s *Simulation) RequireNameAvailable(name string) { s.setup.RequireNameAvailable(name) }
