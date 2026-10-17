package endpoint

import (
	"fmt"
	"github.com/sarchlab/akita/v5/sim/messaging"
	"github.com/sarchlab/akita/v5/sim/modeling"
	"io"
)

// networkPeer derives the destination from a completed point-to-point link.
func networkPeer(c *Comp) messaging.RemotePort {
	port := c.Ports.NetworkPort
	connection, ok := port.Connection().(interface{ Ports() []messaging.Port })
	if !ok {
		panic(c.Name() + ": endpoint: network link must expose its bound ports")
	}
	ports := connection.Ports()
	if len(ports) != 2 {
		panic(c.Name() + ": endpoint: network link must have exactly two ports")
	}
	for i, p := range ports {
		if p == port {
			peer := ports[1-i]
			if peer.Owner() == nil || peer.Connection() != port.Connection() {
				break
			}
			return peer.AsRemote()
		}
	}
	panic(c.Name() + ": endpoint: inconsistent network link")
}

type topology struct{ comp *Comp }

func (t *topology) Name() string { return t.comp.Name() + ".Wiring" }

type topologyRecord struct {
	Devices []messaging.RemotePort
	Peer    messaging.RemotePort
}

func (t *topology) record() topologyRecord {
	record := topologyRecord{Peer: networkPeer(t.comp)}
	seen := make(map[messaging.RemotePort]bool)
	for _, p := range t.comp.Resources.devices.ports {
		name := p.AsRemote()
		if name == "" || seen[name] || p.Connection() != t.comp.Resources.devices {
			panic(fmt.Sprintf("%s: endpoint: inconsistent device binding %s", t.comp.Name(), name))
		}
		seen[name] = true
		record.Devices = append(record.Devices, name)
	}
	return record
}
func (t *topology) SaveCheckpoint(w io.Writer) error {
	return modeling.WriteCheckpoint(w, t.record(), struct{}{}, nil)
}
func (t *topology) LoadCheckpoint(r io.Reader) error {
	return modeling.ReadCheckpoint(r, t.record(), &struct{}{}, nil)
}
