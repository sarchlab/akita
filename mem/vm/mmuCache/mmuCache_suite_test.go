package mmuCache

import (
	"testing"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/sarchlab/akita/v5/hooking"
	"github.com/sarchlab/akita/v5/messaging"
	"github.com/sarchlab/akita/v5/modeling"
	"github.com/sarchlab/akita/v5/timing"
)

func TestMMUCache(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "MMUCache Suite")
}

// noopConn is a minimal messaging.Connection used to drive a component's real
// ports in isolation. Because the mmuCache now owns its ports (they are no
// longer injectable), tests feed requests with Deliver and read responses with
// RetrieveOutgoing; the port still needs a connection so its send/retrieve
// notifications have somewhere to go.
type noopConn struct {
	hooking.HookableBase
}

func (c *noopConn) Name() string                     { return "NoopConn" }
func (c *noopConn) PlugIn(port messaging.Port)       { port.SetConnection(c) }
func (c *noopConn) Unplug(_ messaging.Port)          {}
func (c *noopConn) NotifyAvailable(_ messaging.Port) {}
func (c *noopConn) NotifySend()                      {}

// assignPort builds a port with the given buffer size using the same simulation
// the component was built with, and assigns it to the component's declared port
// of the same name.
func assignPort(
	sim timing.Simulation,
	comp *Comp,
	name string,
	bufSize int,
) messaging.Port {
	p := modeling.MakePortBuilder().
		WithSimulation(sim).
		WithComponent(comp).
		WithSpec(modeling.PortSpec{BufSize: bufSize}).
		Build(name)
	comp.AssignPort(name, p)
	return p
}

// assignDefaultPorts assigns the mmuCache's three declared ports (Top, Bottom,
// Control), each with a buffer of 16 (the historical default).
func assignDefaultPorts(sim timing.Simulation, comp *Comp) {
	assignPort(sim, comp, "Top", 16)
	assignPort(sim, comp, "Bottom", 16)
	assignPort(sim, comp, "Control", 16)
}
