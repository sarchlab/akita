package simulation

import (
	"github.com/rs/xid"
	"github.com/sarchlab/akita/v4/datarecording"
	"github.com/sarchlab/akita/v4/monitoring"
	"github.com/sarchlab/akita/v4/sim"
	"github.com/sarchlab/akita/v4/tracing"
)

// Builder can be used to build a simulation.
type Builder struct {
	parallelEngine bool
	monitorOn      bool
	monitorPort    int
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

// WithMonitorPort sets the port number for the monitoring server.
func (b Builder) WithMonitorPort(port int) Builder {
	b.monitorPort = port
	return b
}

func (b Builder) parametersMustBeValid() {
	if !b.monitorOn && b.monitorPort != 0 {
		panic("monitor port cannot be set when monitoring is disabled")
	}
}

// Build builds the simulation.
func (b Builder) Build() *Simulation {
	b.parametersMustBeValid()

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

	s.visTracer = tracing.NewDBTracer(s.engine, s.dataRecorder)

	if b.monitorOn {
		s.monitor = monitoring.NewMonitor()
		if b.monitorPort > 0 {
			s.monitor.WithPortNumber(b.monitorPort)
		}
		s.monitor.RegisterEngine(s.engine)
		s.monitor.RegisterVisTracer(s.visTracer)
		s.monitor.StartServer()
	}

	return s
}
