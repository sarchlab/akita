package simulation

import (
	"github.com/rs/xid"
	"github.com/sarchlab/akita/v4/datarecording"
	"github.com/sarchlab/akita/v4/monitoring"
	"github.com/sarchlab/akita/v4/sim"
	"github.com/sarchlab/akita/v4/instrumentation/tracing/tracers"
)

// Builder can be used to build a simulation.
type Builder struct {
	parallelEngine bool
	monitorOn      bool
	outputFileName string
}

// MakeBuilder creates a new builder.
func MakeBuilder() Builder {
	return Builder{
		parallelEngine: false,
		monitorOn:      true,
	}
}

// WithParallelEngine sets the simulation to use a parallel engine.
func (b Builder) WithParallelEngine() Builder {
	b.parallelEngine = true
	return b
}

// WithoutMonitoring sets the simulation to not use monitoring.
func (b Builder) WithoutMonitoring() Builder {
	b.monitorOn = false
	return b
}

// WithOutputFileName sets the custom output file name for the data recorder.
func (b Builder) WithOutputFileName(filename string) Builder {
	b.outputFileName = filename
	return b
}

// Build builds the simulation.
func (b Builder) Build() *Simulation {
	s := &Simulation{
		compNameIndex: make(map[string]int),
		portNameIndex: make(map[string]int),
	}

	s.id = xid.New().String()
	
	outputPath := b.outputFileName
	if outputPath == "" {
		outputPath = "akita_sim_" + s.id
	}
	s.dataRecorder = datarecording.NewDataRecorder(outputPath)

	s.engine = sim.NewSerialEngine()
	if b.parallelEngine {
		s.engine = sim.NewParallelEngine()
	}

	if b.monitorOn {
		s.monitor = monitoring.NewMonitor()
		s.monitor.RegisterEngine(s.engine)
		s.monitor.StartServer()
	}

	s.visTracer = tracers.NewDBTracer(s.engine, s.dataRecorder)

	return s
}
