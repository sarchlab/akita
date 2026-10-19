package sim

import (
	"errors"
	"fmt"
	"reflect"
	"slices"
	"sync"

	"github.com/sarchlab/akita/v5/internal/assembly"
	"github.com/sarchlab/akita/v5/sim/datarecording"
	"github.com/sarchlab/akita/v5/sim/naming"
	"github.com/sarchlab/akita/v5/sim/tasks"
	"github.com/sarchlab/akita/v5/sim/tasks/tracing"
	"github.com/sarchlab/akita/v5/sim/timing"
)

type Simulation struct {
	// lifecycle serializes snapshots and cleanup. activeOperation spans callbacks
	// made by Initialize and Run, without holding the mutex during those callbacks.
	// Engine.WithIdle also excludes calls made directly through Engine().
	lifecycle        sync.Mutex
	activeOperation  string
	terminated       bool
	terminateErr     error
	setup            assembly.Setup
	id               string
	outputPath       string
	engine           timing.Engine
	idGenerator      *timing.IDGenerator
	dataRecorder     datarecording.DataRecorder
	visTracer        *tracing.DBTracer
	taskTracker      *tasks.Tracker
	metaRecorder     *metaRecorder
	topologyRecorder *topologyRecorder
	monitor          Monitor
	monitorStarted   bool

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
// can write its own tables into the same recording. It is nil unless recording
// was explicitly enabled during construction.
func (s *Simulation) DataRecorder() datarecording.DataRecorder {
	return s.dataRecorder
}

// VisTracer returns the visualization tracer owned by the simulation.
// It returns nil unless both task tracking and recording are enabled. For manual
// StartTracing, use WithTaskTracking and WithOutputPath. WithMonitor and
// WithVisTracingOnStart enable both services.
func (s *Simulation) VisTracer() *tracing.DBTracer {
	return s.visTracer
}

// TraceDBPath returns the exact SQLite recording path, or "" without recording.
func (s *Simulation) TraceDBPath() string {
	return s.outputPath
}

// Components returns a copy of the registered component inventory, in registration order.
// The objects themselves remain live. Inventory is frozen after initialization.
func (s *Simulation) Components() []Component { return slices.Clone(s.components) }

// Ports returns a copy of the registered port inventory, in registration order.
// The objects themselves remain live. Inventory is frozen after initialization.
func (s *Simulation) Ports() []Port { return slices.Clone(s.ports) }

// registerEntity records a live entity in the single, flat inventory. It is the
// only cross-kind uniqueness check — names must be globally unique across all
// kinds, which is what makes the inventory a well-defined state snapshot. The
// typed Register methods pass the concrete object, which satisfies Entity, so
// the inventory holds the live entity itself.
func (s *Simulation) registerEntity(e Entity) {
	if e.Name() == taskTrackerCheckpointName {
		if _, ok := e.(*taskTrackerCheckpoint); !ok {
			panic("simulation: TaskTracker is a reserved entity name")
		}
	}
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
}

// RegisterPort registers a port with the simulation so it can be resolved by
// name and monitored. It also supplies task tracking to ports that support it.
// A component model's BindPort registers each port it binds;
// an owner written without a component model registers its own ports.
func (s *Simulation) RegisterPort(p naming.Named) {
	port, ok := p.(Port)
	if !ok {
		panic("simulation: RegisterPort requires a messaging.Port, got " +
			p.Name())
	}

	// Check setup and name collisions before binding. Binding also prevents
	// a port from moving between simulations.
	s.RequireNameAvailable(port.Name())
	if tracked, ok := port.(interface{ BindTaskTracker(*tasks.Tracker) }); ok {
		tracked.BindTaskTracker(s.taskTracker)
	}
	s.registerEntity(port)
	s.ports = append(s.ports, port)
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

// Terminate releases simulation services exactly once. Active execution,
// including a paused Run, must return first. Termination is final even if
// cleanup fails; repeated calls return the original result.
func (s *Simulation) Terminate() error {
	s.lifecycle.Lock()
	defer s.lifecycle.Unlock()
	if s.terminated {
		return s.terminateErr
	}
	if s.activeOperation != "" {
		return fmt.Errorf("simulation: %s is active", s.activeOperation)
	}
	return s.engine.WithIdle(func() error {
		s.terminated = true
		s.terminateErr = s.cleanup()
		return s.terminateErr
	})
}

func (s *Simulation) cleanup() (err error) {
	// Legacy cleanup operations may panic; retain the failure as the terminal
	// result and still attempt to close the owned recorder.
	defer func() {
		if cause := recover(); cause != nil {
			err = errors.Join(err, fmt.Errorf("simulation: cleanup panic: %v", cause))
		}
	}()
	if s.dataRecorder != nil {
		defer func() { err = errors.Join(err, s.dataRecorder.Close()) }()
	}
	if s.monitorStarted {
		s.monitor.Stop()
		s.monitorStarted = false
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

	return nil
}

// NewID allocates an ID unique within this simulation.
func (s *Simulation) NewID() uint64 { return s.idGenerator.NewID() }

// Initialize validates the completed wiring, freezes topology, and creates state
// and middleware. Call it once before seeding work or loading a checkpoint.
func (s *Simulation) Initialize() error {
	s.lifecycle.Lock()
	if err := s.requireIdle(); err != nil {
		s.lifecycle.Unlock()
		return err
	}
	s.activeOperation = "initialization"
	s.lifecycle.Unlock()
	defer s.finishOperation()
	if err := s.setup.Initialize(); err != nil {
		return err
	}
	if s.monitor != nil {
		s.monitor.Start(s)
		s.monitorStarted = true
	}
	return nil
}

// Run executes queued events after successful initialization.
func (s *Simulation) Run() error {
	s.lifecycle.Lock()
	if err := s.requireIdle(); err != nil {
		s.lifecycle.Unlock()
		return err
	}
	if err := s.setup.Ready(); err != nil {
		s.lifecycle.Unlock()
		return err
	}
	s.activeOperation = "execution"
	s.lifecycle.Unlock()
	defer s.finishOperation()
	return s.engine.Run()
}

func (s *Simulation) requireIdle() error {
	if s.terminated {
		return fmt.Errorf("simulation: terminated")
	}
	if s.activeOperation != "" {
		return fmt.Errorf("simulation: %s is active", s.activeOperation)
	}
	return nil
}

func (s *Simulation) finishOperation() {
	s.lifecycle.Lock()
	s.activeOperation = ""
	s.lifecycle.Unlock()
}

func (s *Simulation) whileIdle(fn func() error) error {
	s.lifecycle.Lock()
	defer s.lifecycle.Unlock()
	if err := s.requireIdle(); err != nil {
		return err
	}
	return s.engine.WithIdle(fn)
}

// RequireSetup rejects structural changes after initialization begins.
func (s *Simulation) RequireSetup() { s.setup.RequireSetup() }

// RequireNameAvailable validates a name before a binding changes either side.
func (s *Simulation) RequireNameAvailable(name string) { s.setup.RequireNameAvailable(name) }

// TaskTracker returns the simulation task service, or nil when tracking is disabled.
func (s *Simulation) TaskTracker() *tasks.Tracker { return s.taskTracker }

// ownsEntity checks object identity as well as the globally unique name.
func (s *Simulation) ownsEntity(named naming.Named) bool {
	i, ok := s.entityByName[named.Name()]
	return ok && reflect.TypeOf(named).Comparable() && s.entities[i] == named
}
