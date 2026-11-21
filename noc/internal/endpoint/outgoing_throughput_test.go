package endpoint

import (
	"fmt"
	"testing"

	"github.com/sarchlab/akita/v5/mem/memprotocol"
	"github.com/sarchlab/akita/v5/noc/packetization"
	akitasim "github.com/sarchlab/akita/v5/sim"
	"github.com/sarchlab/akita/v5/sim/messaging"
	"github.com/sarchlab/akita/v5/sim/messaging/twowaybuffered"
	"github.com/sarchlab/akita/v5/sim/modeling/modelingtest"
	"github.com/stretchr/testify/require"
)

// wakeCountingOwner records how often a blocked sender would be woken.
type wakeCountingOwner struct {
	payloadOwner
	wakes int
}

func (o *wakeCountingOwner) NotifyPortFree(messaging.Port) { o.wakes++ }

type outgoingFixture struct {
	sim     interface{ NewID() uint64 }
	ep      *Comp
	network messaging.Port
	devices []messaging.Port
	owners  []*wakeCountingOwner
}

// newOutgoingFixture builds an endpoint whose network port is drained by hand,
// so each test controls link backpressure cycle by cycle.
func newOutgoingFixture(width, numPorts, deviceCapacity int) *outgoingFixture {
	s := akitasim.MakeBuilder().WithTaskTracking().Build()
	network := twowaybuffered.MakeBuilder().WithSimulation(s).
		WithSpec(twowaybuffered.Spec{IncomingCapacity: width, OutgoingCapacity: width}).
		Build()

	spec := DefaultSpec()
	spec.NumOutputChannels = width
	spec.FlitByteSize = 32
	spec.EncodingOverhead = 0
	ep := MakeBuilder().WithSimulation(s).WithSpec(spec).Build("Endpoint")
	ep.BindPort("NetworkPort", network)

	f := &outgoingFixture{sim: s, ep: ep, network: network}
	for i := range numPorts {
		owner := &wakeCountingOwner{payloadOwner: payloadOwner{fmt.Sprintf("Device%d", i)}}
		port := twowaybuffered.MakeBuilder().WithSimulation(s).
			WithSpec(twowaybuffered.Spec{IncomingCapacity: 1, OutgoingCapacity: deviceCapacity}).
			Build()
		port.BindOwner(owner, owner.Name()+".Port")
		DeviceConnection(ep).BindPort(port)
		f.devices = append(f.devices, port)
		f.owners = append(f.owners, owner)
	}
	ep.Resources.Peer = connectPeer(s, network)
	if err := s.Initialize(); err != nil {
		panic(err)
	}

	return f
}

// send queues a message of the given size on a device port and returns its ID.
func (f *outgoingFixture) send(port, trafficBytes int) uint64 {
	id := f.sim.NewID()
	f.devices[port].Send(messaging.Msg{
		ID:           id,
		Src:          f.devices[port].AsRemote(),
		Dst:          "Receiver.Port",
		TrafficBytes: trafficBytes,
		Payload:      memprotocol.ReadReq{Address: id, AccessByteSize: 4},
	})

	return id
}

// drainLink removes up to limit flits from the network port's outgoing buffer.
func (f *outgoingFixture) drainLink(limit int) []packetization.Flit {
	var flits []packetization.Flit
	for len(flits) < limit {
		msg, ok := f.network.RetrieveOutgoing()
		if !ok {
			break
		}
		flits = append(flits, msg.Payload.(packetization.Flit))
	}

	return flits
}

func msgOutBufIDs(ep *Comp) []uint64 {
	ids := make([]uint64, 0, len(ep.State.MsgOutBuf))
	for _, msg := range ep.State.MsgOutBuf {
		ids = append(ids, msg.ID)
	}

	return ids
}

// requireSustainedInjection checks that a backlogged port puts width one-flit
// messages on the link every cycle, in order.
func requireSustainedInjection(t *testing.T, width int) {
	t.Helper()

	const cycles = 8
	f := newOutgoingFixture(width, 1, width*(cycles+2))
	want := make([]uint64, 0, width*(cycles+2))
	for range width * (cycles + 2) {
		want = append(want, f.send(0, 32))
	}

	// The first tick only fills the buffers; the link is busy from then on.
	modelingtest.Tick(f.ep)

	got := make([]uint64, 0, width*cycles)
	for cycle := range cycles {
		modelingtest.Tick(f.ep)
		flits := f.drainLink(width + 1)
		require.Lenf(t, flits, width, "cycle %d", cycle)
		for _, flit := range flits {
			got = append(got, flit.MsgID)
		}
	}
	require.Equal(t, want[:width*cycles], got)
}

func TestBackloggedPortFillsLinkEveryCycle(t *testing.T) {
	for _, width := range []int{1, 2, 4, 8, 16} {
		t.Run(fmt.Sprintf("width-%d", width), func(t *testing.T) {
			requireSustainedInjection(t, width)
		})
	}
}

func TestInternalBuffersDoNotLimitWideLinks(t *testing.T) {
	// 17 and 32 exceed the message buffer; 65 and 128 exceed both buffers.
	for _, width := range []int{17, 32, 64, 65, 128} {
		t.Run(fmt.Sprintf("width-%d", width), func(t *testing.T) {
			requireSustainedInjection(t, width)
		})
	}
}

func TestWidthOneTakesOneMessagePerPortPerTick(t *testing.T) {
	f := newOutgoingFixture(1, 2, 4)
	a0, b0 := f.send(0, 32), f.send(1, 32)
	a1, b1 := f.send(0, 32), f.send(1, 32)

	require.True(t, f.ep.Middlewares.Outgoing.prepareMsg())
	require.Equal(t, []uint64{a0, b0}, msgOutBufIDs(f.ep))

	require.True(t, f.ep.Middlewares.Outgoing.prepareMsg())
	require.Equal(t, []uint64{a0, b0, a1, b1}, msgOutBufIDs(f.ep))
}

func TestBackloggedPortsAreServedAlternately(t *testing.T) {
	f := newOutgoingFixture(3, 2, 8)
	a, b := make([]uint64, 0, 4), make([]uint64, 0, 4)
	for range 4 {
		a = append(a, f.send(0, 32))
		b = append(b, f.send(1, 32))
	}

	require.True(t, f.ep.Middlewares.Outgoing.prepareMsg())
	require.Equal(t, []uint64{a[0], b[0], a[1], b[1], a[2], b[2]}, msgOutBufIDs(f.ep))
}

func TestPassesContinueAfterOnePortEmpties(t *testing.T) {
	f := newOutgoingFixture(3, 2, 8)
	a0 := f.send(0, 32)
	b0, b1, b2 := f.send(1, 32), f.send(1, 32), f.send(1, 32)

	require.True(t, f.ep.Middlewares.Outgoing.prepareMsg())
	require.Equal(t, []uint64{a0, b0, b1, b2}, msgOutBufIDs(f.ep))
}

func TestPassesStopAtMessageBufferLimit(t *testing.T) {
	f := newOutgoingFixture(16, 2, 16)
	for range 16 {
		f.send(0, 32)
		f.send(1, 32)
	}

	require.True(t, f.ep.Middlewares.Outgoing.prepareMsg())
	require.Len(t, f.ep.State.MsgOutBuf, maxMsgOutBufSize)
	require.Equal(t, 8, f.devices[0].OutgoingBuf().Size())
	require.Equal(t, 8, f.devices[1].OutgoingBuf().Size())

	require.False(t, f.ep.Middlewares.Outgoing.prepareMsg())
}

func TestMultiFlitMessagesSurviveBackpressure(t *testing.T) {
	const (
		width      = 4
		numPorts   = 3
		msgPerPort = 40
	)
	f := newOutgoingFixture(width, numPorts, msgPerPort)
	portOf := map[uint64]int{}
	want := make([][]uint64, numPorts)
	numFlits := map[uint64]int{}
	for i := range msgPerPort {
		for port := range numPorts {
			size := 1 + (i+port)%5
			id := f.send(port, size*32)
			portOf[id] = port
			numFlits[id] = size
			want[port] = append(want[port], id)
		}
	}

	got := make([][]uint64, numPorts)
	nextSeq := map[uint64]int{}
	// Vary link availability, including fully blocked cycles.
	drain := []int{0, 1, 4, 0, 0, 2, 4, 3}
	for cycle := 0; cycle < 5000; cycle++ {
		modelingtest.Tick(f.ep)
		for _, flit := range f.drainLink(drain[cycle%len(drain)]) {
			require.Equal(t, numFlits[flit.MsgID], flit.NumFlitInMsg)
			require.Equal(t, nextSeq[flit.MsgID], flit.SeqID, "flits of a message reordered")
			nextSeq[flit.MsgID]++
			if flit.SeqID == 0 {
				got[portOf[flit.MsgID]] = append(got[portOf[flit.MsgID]], flit.MsgID)
			}
		}
		require.LessOrEqual(t, len(f.ep.State.MsgOutBuf), maxMsgOutBufSize)
	}

	require.Equal(t, want, got)
	for id, n := range numFlits {
		require.Equalf(t, n, nextSeq[id], "message %d lost flits", id)
	}
	require.Empty(t, f.ep.State.MsgOutBuf)
	require.Empty(t, f.ep.State.FlitsToSend)
}

func TestBlockedDeviceSenderIsWokenWhenRoomAppears(t *testing.T) {
	f := newOutgoingFixture(4, 1, 2)
	f.send(0, 32)
	f.send(0, 32)
	require.False(t, f.devices[0].CanSend())

	require.True(t, f.ep.Middlewares.Outgoing.prepareMsg())
	require.True(t, f.devices[0].CanSend())
	require.Equal(t, 1, f.owners[0].wakes)
}
