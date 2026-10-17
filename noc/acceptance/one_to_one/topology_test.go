package main

import (
	"bytes"
	"testing"

	"github.com/sarchlab/akita/v5/sim/messaging"
	"github.com/sarchlab/akita/v5/sim/messaging/twowaybuffered"
	"github.com/stretchr/testify/require"
)

type topologyOwner struct{}

func (topologyOwner) Name() string                  { return "Owner" }
func (topologyOwner) NotifyRecv(messaging.Port)     {}
func (topologyOwner) NotifyPortFree(messaging.Port) {}
func TestDirectNetworkFingerprint(t *testing.T) {
	port := func(name string) messaging.Port {
		p := twowaybuffered.NewPort(1, 1)
		p.BindOwner(topologyOwner{}, name)
		return p
	}
	a, b, c := port("A"), port("B"), port("C")
	n := &networkTopology{devices: [][]messaging.Port{{a, b}, {c}}, peers: []messaging.Port{port("EP1"), port("EP2")}}
	var saved bytes.Buffer
	require.NoError(t, n.SaveCheckpoint(&saved))
	require.NoError(t, n.LoadCheckpoint(bytes.NewReader(saved.Bytes())))
	n.devices[0] = []messaging.Port{b, a}
	require.ErrorContains(t, n.LoadCheckpoint(bytes.NewReader(saved.Bytes())), "spec hash mismatch")
	n.devices[0] = []messaging.Port{a, b}
	n.peers[0], n.peers[1] = n.peers[1], n.peers[0]
	require.ErrorContains(t, n.LoadCheckpoint(bytes.NewReader(saved.Bytes())), "spec hash mismatch")
	t.Log("One direct-network fingerprint restores matching topology and rejects changed device order or peer pairing")
}
