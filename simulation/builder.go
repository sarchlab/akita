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

// Build builds the simulation.
func (b Builder) Build() *Simulation {
	s := &Simulation{
		compNameIndex: make(map[string]int),
		portNameIndex: make(map[string]int),
	}

	s.id = xid.New().String()
	s.dataRecorder = datarecording.NewDataRecorder("akita_sim_" + s.id)

	s.engine = sim.NewSerialEngine()
	if b.parallelEngine {
		s.engine = sim.NewParallelEngine()
	}

	if b.monitorOn {
		s.monitor = monitoring.NewMonitor()
		s.monitor.RegisterEngine(s.engine)
		s.monitor.StartServer()
	}

	s.visTracer = tracing.NewDBTracer(s.engine, s.dataRecorder)

	return s
}
