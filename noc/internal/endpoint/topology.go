package endpoint

import "github.com/sarchlab/akita/v5/sim/messaging"

// networkPeer reads the explicitly supplied peer after wiring is complete.
func networkPeer(c *Comp) messaging.RemotePort {
	peer := c.Resources.Peer
	if peer == nil || peer.Owner() == nil || c.Ports.NetworkPort.Connection() == nil ||
		peer.Connection() != c.Ports.NetworkPort.Connection() {
		panic(c.Name() + ": endpoint: inconsistent network peer")
	}
	return peer.AsRemote()
}
