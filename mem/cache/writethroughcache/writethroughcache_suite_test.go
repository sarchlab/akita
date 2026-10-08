package writethroughcache

import (
	"log"
	"testing"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/sarchlab/akita/v5/sim/hooking"
	"github.com/sarchlab/akita/v5/sim/messaging"
	"github.com/sarchlab/akita/v5/sim/messaging/twowaybuffered"
	"github.com/sarchlab/akita/v5/sim/modeling"
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
func (c *noopConn) NotifySend()                      {}

// makePorts creates the Top, Bottom, and Control ports of the cache named
// name, each with the given buffer size.
func makePorts(name string, bufSize int) Ports {
	return Ports{
		Top:     twowaybuffered.NewPort(bufSize, bufSize),
		Bottom:  twowaybuffered.NewPort(bufSize, bufSize),
		Control: twowaybuffered.NewPort(bufSize, bufSize),
	}
}

// buildStageTestCache builds a cache named "Cache" for a stage test from the
// given spec, resources, and ports, and replaces its State with state. It
// returns the pipeline middleware the stages under test belong to.
func buildStageTestCache(
	spec Spec, res Resources, ports Ports, state state,
) *pipelineMW {
	setupSim1 := modeling.NewStandaloneSimulation(timing.NewSerialEngine())
	comp := Definition.Builder().
		WithSimulation(setupSim1).
		WithSpec(spec).
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

	return comp.Middlewares.Pipeline
}

func TestWriteThroughCache(t *testing.T) {
	log.SetOutput(GinkgoWriter)
	RegisterFailHandler(Fail)
	RunSpecs(t, "WriteThroughCache Suite")
}

func TestValidateState(t *testing.T) {
	if err := modeling.ValidateState(state{}); err != nil {
		t.Fatalf("State failed validation: %v", err)
	}
}
