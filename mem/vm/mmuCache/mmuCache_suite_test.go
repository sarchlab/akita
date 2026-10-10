package mmuCache

import (
	"testing"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/sarchlab/akita/v5/sim/hooking"
	"github.com/sarchlab/akita/v5/sim/messaging"
	"github.com/sarchlab/akita/v5/sim/messaging/twowaybuffered"
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
func (c *noopConn) BindPort(port messaging.Port)     { port.BindConnection(c) }
func (c *noopConn) Unplug(_ messaging.Port)          {}
func (c *noopConn) NotifyAvailable(_ messaging.Port) {}
func (c *noopConn) NotifySend()                      {}

// defaultPorts creates the three ports (Top, Bottom, Control) of the mmuCache
// named name, each with a buffer of 16 (the historical default).
func defaultPorts(name string) Ports {
	return Ports{
		Top:     twowaybuffered.NewPort(16, 16),
		Bottom:  twowaybuffered.NewPort(16, 16),
		Control: twowaybuffered.NewPort(16, 16),
	}
}
