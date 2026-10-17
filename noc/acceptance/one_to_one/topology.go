package main

import (
	"io"

	"github.com/sarchlab/akita/v5/sim/messaging"
	"github.com/sarchlab/akita/v5/sim/modeling"
)

// This example assembles its own network without a connector, so it registers
// one record for the two endpoints' ordered devices and network-port pairing.
type networkTopology struct {
	devices [][]messaging.Port
	peers   []messaging.Port
}

func (*networkTopology) Name() string { return "DirectNetwork.Topology" }
func (n *networkTopology) record() [][]messaging.RemotePort {
	record := make([][]messaging.RemotePort, len(n.devices))
	for i, devices := range n.devices {
		record[i] = append(record[i], n.peers[i].AsRemote(), n.peers[1-i].AsRemote())
		for _, p := range devices {
			record[i] = append(record[i], p.AsRemote())
		}
	}
	return record
}
func (n *networkTopology) SaveCheckpoint(w io.Writer) error {
	return modeling.WriteCheckpoint(w, n.record(), struct{}{}, nil)
}
func (n *networkTopology) LoadCheckpoint(r io.Reader) error {
	return modeling.ReadCheckpoint(r, n.record(), &struct{}{}, nil)
}
