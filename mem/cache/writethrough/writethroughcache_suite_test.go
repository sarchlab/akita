package writethrough

import (
	"testing"

	"github.com/sarchlab/akita/v5/internal/testsim"
	akitasim "github.com/sarchlab/akita/v5/sim"
	"github.com/sarchlab/akita/v5/sim/hooking"
	"github.com/sarchlab/akita/v5/sim/messaging"
	"github.com/sarchlab/akita/v5/sim/messaging/twowaybuffered"
	"github.com/sarchlab/akita/v5/sim/modeling"
	"github.com/sarchlab/akita/v5/sim/queueing"
	"github.com/sarchlab/akita/v5/sim/timing"
)

// noopConn is a minimal messaging.Connection used to drive a component's real
// ports in isolation. Stages own real ports (no longer mock-injected), so tests
// drive them with Deliver and read results with RetrieveOutgoing; the port
// still needs a connection so its send/retrieve notifications have somewhere to
// go.
type noopConn struct {
	hooking.HookableBase
}

func (c *noopConn) Name() string                     { return "NoopConn" }
func (c *noopConn) BindPort(port messaging.Port)     { port.BindConnection(c) }
func (c *noopConn) Unplug(_ messaging.Port)          {}
func (c *noopConn) NotifyAvailable(_ messaging.Port) {}
func (c *noopConn) NotifySend(_ messaging.Port)      {}

// makePorts creates the Top, Bottom, and Control ports of the cache named
// name, each with the given buffer size.
func makePorts(simulation modeling.Simulation, name string, bufSize int) Ports {
	return Ports{
		Top: twowaybuffered.MakeBuilder().
			WithSimulation(simulation).
			WithSpec(twowaybuffered.Spec{IncomingCapacity: bufSize, OutgoingCapacity: bufSize}).
			Build(),
		Bottom: twowaybuffered.MakeBuilder().
			WithSimulation(simulation).
			WithSpec(twowaybuffered.Spec{IncomingCapacity: bufSize, OutgoingCapacity: bufSize}).
			Build(),
		Control: twowaybuffered.MakeBuilder().
			WithSimulation(simulation).
			WithSpec(twowaybuffered.Spec{IncomingCapacity: bufSize, OutgoingCapacity: bufSize}).
			Build(),
	}
}

// buildStageTestCache builds a cache named "Cache" for a stage test from the
// given spec, resources, and ports, and replaces its State with state. It
// returns the pipeline middleware the stages under test belong to.
func buildStageTestCache(setupSim1 *akitasim.Simulation,
	spec Spec, res Resources, ports Ports, state state,
) *pipelineMW {
	comp := modeling.MakeBuilder[*Comp](spec).
		WithSimulation(setupSim1).
		WithResources(res).
		Build("Cache")
	comp.BindPort("Top", ports.Top)
	comp.BindPort("Bottom", ports.Bottom)
	comp.BindPort("Control", ports.Control)
	for _, p := range []messaging.Port{ports.Top, ports.Bottom, ports.Control} {
		(&noopConn{}).BindPort(p)
	}
	if err := setupSim1.Initialize(); err != nil {
		panic(err)
	}

	comp.State = state
	comp.State.AfterCheckpointLoad(comp)

	return comp.Middlewares.Pipeline
}

// stageBankPipeline and stageDirPipeline create pipelines for a hand-built
// State. buildStageTestCache gives them the component's clocks.
func stageBankPipeline(width, numStages int) queueing.FastPipeline[int] {
	return queueing.MakeFastPipeline[int](width, numStages, 1*timing.GHz, testsim.NewClock())
}

func stageDirPipeline(width, numStages int) queueing.FastPipeline[int] {
	return queueing.MakeFastPipeline[int](width, numStages, dirStepFreq, testsim.NewClock())
}

func TestValidateState(t *testing.T) {
	if err := modeling.ValidateState(state{}); err != nil {
		t.Fatalf("State failed validation: %v", err)
	}
}
