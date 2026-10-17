package endpoint

import (
	"github.com/sarchlab/akita/v5/sim/hooking"
	"github.com/sarchlab/akita/v5/sim/messaging"
	"github.com/sarchlab/akita/v5/sim/messaging/direct"
	"github.com/sarchlab/akita/v5/sim/messaging/twowaybuffered"
	"github.com/sarchlab/akita/v5/sim/timing"
)

type testLink struct {
	hooking.HookableBase
	ports []messaging.Port
}

func (*testLink) Name() string                   { return "TestLink" }
func (*testLink) NotifySend()                    {}
func (*testLink) NotifyAvailable(messaging.Port) {}
func (l *testLink) BindPort(p messaging.Port)    { p.BindConnection(l); l.ports = append(l.ports, p) }
func (l *testLink) Ports() []messaging.Port      { return l.ports }
func connectPeer(s timing.Simulation, port messaging.Port) {
	peer := twowaybuffered.NewPort(1, 1)
	peer.BindOwner(payloadOwner{"Peer"}, "Peer.Port")
	s.RegisterPort(peer)
	conn := direct.NewConnection("PeerConnection", s, timing.GHz)
	conn.BindPort(port)
	conn.BindPort(peer)
}
