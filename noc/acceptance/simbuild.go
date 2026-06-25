package acceptance

import (
	"flag"

	"github.com/sarchlab/akita/v5/simulation"
)

// traceFlag enables vis-trace collection. It is registered here so every
// acceptance main (all of which import this package) exposes a uniform -trace
// flag. It is read by NewSimulation, which must be called after flag.Parse().
var traceFlag = flag.Bool("trace", false,
	"Collect a vis trace (plus simulator source and topology) into trace.sqlite3")

// NewSimulation builds the Simulation that every acceptance main runs on, so
// the network's components register with a real simulation object. Monitoring
// is disabled to keep the runs headless. With -trace the simulation records a
// full vis trace — plus the simulator source and topology — into trace.sqlite3;
// without it a proper simulation is still built (topology and metadata only).
//
// Call after flag.Parse(), and call Terminate() on the returned simulation once
// the run finishes so the recording is flushed and closed.
func NewSimulation() *simulation.Simulation {
	b := simulation.MakeBuilder().WithoutMonitoring()

	if *traceFlag {
		b = b.WithVisTracingOnStart().WithOutputFileName("trace")
	}

	return b.Build()
}
