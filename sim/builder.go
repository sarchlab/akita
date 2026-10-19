package sim

import (
	"io/fs"
	"maps"

	"github.com/rs/xid"
	"github.com/sarchlab/akita/v5/internal/simconfig"
	"github.com/sarchlab/akita/v5/sim/datarecording"
	"github.com/sarchlab/akita/v5/sim/naming"
	"github.com/sarchlab/akita/v5/sim/tasks"
	"github.com/sarchlab/akita/v5/sim/tasks/tracing"
	"github.com/sarchlab/akita/v5/sim/timing"
)

type engineOption = simconfig.EngineOption

// Builder can be used to build a simulation.
type Builder struct {
	engineOption
	parallelEngine    bool
	monitor           Monitor
	outputPath        string
	visTracingOnStart bool
	taskTracking      bool
	recordSource      bool
	sourceFSes        map[string]fs.FS
}

// MakeBuilder creates a new builder.
func MakeBuilder() Builder {
	return Builder{
		parallelEngine: false,
		recordSource:   true,
	}
}

// WithParallelEngine sets the simulation to use a parallel engine.
func (b Builder) WithParallelEngine() Builder {
	b.parallelEngine = true
	return b
}

// WithOutputPath enables recording at the exact path. Build panics if the file
// already exists; it never overwrites an existing recording.
func (b Builder) WithOutputPath(path string) Builder {
	if path == "" {
		panic("simulation: output path cannot be empty")
	}
	b.outputPath = path
	return b
}

// WithMonitor attaches a monitor. Successful Initialize calls Start after all
// component state is initialized and topology is frozen. Terminate stops the
// monitor before closing recording resources.
// Each monitor belongs to one simulation and must not be reused.
// Omitting WithMonitor leaves monitoring disabled.
func (b Builder) WithMonitor(m Monitor) Builder {
	b.monitor = m
	return b
}

// WithVisTracingOnStart enables visual tracing from the start of the simulation.
func (b Builder) WithVisTracingOnStart() Builder {
	b.visTracingOnStart = true
	return b
}

// WithSourceFS registers an additional source tree to record into the trace
// (e.g. a simulator's own components), keyed by a label such as its module
// path. Akita's own source is recorded automatically; use this so DaisenBot can
// also read your components' source. Typically called with a //go:embed FS:
//
//	//go:embed *.go cu/*.go
//	var srcFS embed.FS
//	s := sim.MakeBuilder().WithSourceFS("github.com/me/mysim", srcFS)
//
// Source is only recorded when vis tracing is enabled (the trace is meant for
// DaisenBot). Repeated calls add multiple roots.
func (b Builder) WithSourceFS(root string, fsys fs.FS) Builder {
	next := make(map[string]fs.FS, len(b.sourceFSes)+1)
	maps.Copy(next, b.sourceFSes)
	next[root] = fsys
	b.sourceFSes = next
	return b
}

// WithoutSourceRecording disables recording source into the trace. Source
// recording (Akita's source by default, plus any WithSourceFS roots) is on by
// default for traced simulations; disable it to keep traces minimal.
func (b Builder) WithoutSourceRecording() Builder {
	b.recordSource = false
	return b
}

// Build creates runtime services. Recording and monitoring are opt-in; a plain
// Build creates no files or listeners. Assemble components and call Initialize
// before Run.
func (b Builder) Build() *Simulation {
	s := b.createSimulation()

	b.createDataRecorder(s)
	b.createEngine(s)
	b.createIDGenerator(s)
	b.createMetaRecorder(s)
	b.createSourceRecorder(s)
	b.createTopologyRecorder(s)
	b.createTaskTracker(s)
	b.createVisTracer(s)
	s.monitor = b.monitor

	return s
}

// createSourceRecorder records the simulator source into the trace so it is
// self-describing for DaisenBot. It runs only for traced simulations (the trace
// is the consumer) and can be disabled with WithoutSourceRecording.
func (b Builder) createSourceRecorder(s *Simulation) {
	if !b.recordSource || !b.visTracingOnStart {
		return
	}

	if err := recordSourceArchives(s.dataRecorder, b.sourceFSes); err != nil {
		panic(err)
	}
}

// createTopologyRecorder records the static structure of the simulation — each
// component's spec and the port-level connection graph — into the data
// recording, making it self-describing for tools such as Daisen's index page.
// Like the meta recorder it always runs, independent of visualization tracing,
// so the structure is captured for every simulation that produces a recording.
// The data itself is written at Terminate, once every component, port, and
// connection has been registered.
func (b Builder) createTopologyRecorder(s *Simulation) {
	if s.dataRecorder == nil {
		return
	}
	s.topologyRecorder = newTopologyRecorder(s.dataRecorder)
}

func (b Builder) createSimulation() *Simulation {
	return &Simulation{
		id:           xid.New().String(),
		entityByName: make(map[string]int),
	}
}

func (b Builder) createDataRecorder(s *Simulation) {
	if b.outputPath == "" && !b.visTracingOnStart && b.monitor == nil {
		return
	}
	outputPath := b.outputPath
	if outputPath == "" {
		outputPath = "akita_sim_" + s.id + ".sqlite3"
	}
	s.outputPath = outputPath
	s.dataRecorder = datarecording.NewDataRecorder(outputPath)
}

func (b Builder) createEngine(s *Simulation) {
	if engine := simconfig.Engine(&b.engineOption); engine != nil {
		s.engine = engine
		if named, ok := engine.(naming.Named); ok {
			s.registerEntity(named)
		}
		return
	}

	if b.parallelEngine {
		engine := timing.NewParallelEngine()
		s.engine = engine
		s.registerEntity(engine)
	} else {
		engine := timing.NewSerialEngine()
		s.engine = engine
		s.registerEntity(engine)
	}
}

// createIDGenerator registers this simulation's ID generator as an entity so its
// counter is captured in the state snapshot.
func (b Builder) createIDGenerator(s *Simulation) {
	s.idGenerator = &timing.IDGenerator{}
	s.registerEntity(s.idGenerator)
}

func (b Builder) createMetaRecorder(s *Simulation) {
	if s.dataRecorder == nil {
		return
	}
	s.metaRecorder = newMetaRecorder(s.dataRecorder, s.engine)
}

func (b Builder) createVisTracer(s *Simulation) {
	if s.taskTracker == nil || s.dataRecorder == nil {
		return
	}

	s.visTracer = tracing.NewDBTracer(s.engine, s.dataRecorder)
	s.taskTracker.Subscribe(s.visTracer)

	if b.visTracingOnStart {
		s.visTracer.StartTracing()
	}
}

// WithTaskTracking enables task bookkeeping and observer subscriptions. It is
// also implied by WithVisTracingOnStart and WithMonitor.
func (b Builder) WithTaskTracking() Builder { b.taskTracking = true; return b }

func (b Builder) createTaskTracker(s *Simulation) {
	if b.taskTracking || b.visTracingOnStart || b.monitor != nil {
		s.taskTracker = tasks.NewTracker(s.engine, s.NewID, s.ownsEntity)
		s.registerEntity(&taskTrackerCheckpoint{Tracker: s.taskTracker})
	}
}
