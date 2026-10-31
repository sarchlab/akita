package writethrough

import (
	"testing"

	"github.com/sarchlab/akita/v5/mem"
	"github.com/sarchlab/akita/v5/mem/memprotocol"
	"github.com/sarchlab/akita/v5/sim"
	"github.com/sarchlab/akita/v5/sim/messaging"
	"github.com/stretchr/testify/require"
)

func responseTestCache(width, capacity int) (*sim.Simulation, *pipelineMW) {
	simulation := sim.MakeBuilder().WithTaskTracking().Build()
	spec := DefaultSpec()
	spec.NumReqPerCycle = width
	spec.Bottom.Destinations = []messaging.RemotePort{"Memory.Top"}
	mw := buildStageTestCache(simulation, spec,
		Resources{Storage: mem.NewStorage(4 * mem.KB)},
		makePorts(simulation, "Cache", capacity), state{})
	return simulation, mw
}

func TestRespondStageNoReadyTransactions(t *testing.T) {
	simulation, mw := responseTestCache(4, 4)
	t.Cleanup(func() { require.NoError(t, simulation.Terminate()) })
	mw.comp.State.Transactions = []transactionState{{}, {Done: true, Removed: true}}
	before := simulation.NewID()
	require.False(t, mw.tickRespondStage())
	require.Equal(t, before+1, simulation.NewID())
}

func TestRespondStageBlockedUntilNextTick(t *testing.T) {
	simulation, mw := responseTestCache(4, 1)
	t.Cleanup(func() { require.NoError(t, simulation.Terminate()) })
	mw.comp.State.Transactions = []transactionState{
		{Done: true, HasRead: true, ReadMeta: messaging.Msg{ID: 100, Src: "Source"}},
		{Done: true, HasWrite: true, WriteMeta: messaging.Msg{ID: 101, Src: "Source"}},
	}
	port := mw.topPort()
	port.Send(messaging.Msg{ID: simulation.NewID(), Src: port.AsRemote(),
		Dst: "Source", Payload: memprotocol.WriteDoneRsp{}})
	before := simulation.NewID()
	require.False(t, mw.tickRespondStage())
	require.Equal(t, before+1, simulation.NewID()) // Blocked sends allocate no IDs.
	require.False(t, mw.comp.State.Transactions[0].Removed)
	require.False(t, mw.comp.State.Transactions[1].Removed)

	port.RetrieveOutgoing()
	require.True(t, mw.tickRespondStage())
	rsp, ok := port.RetrieveOutgoing()
	require.True(t, ok)
	require.Equal(t, uint64(100), rsp.RspTo)
	require.False(t, mw.comp.State.Transactions[1].Removed)
	require.True(t, mw.tickRespondStage())
	rsp, ok = port.RetrieveOutgoing()
	require.True(t, ok)
	require.Equal(t, uint64(101), rsp.RspTo)
}

// retryResponsePort simulates a concurrent consumer freeing the outgoing slot
// between attempts. Direct connections do not do this, but NoC endpoints can.
type retryResponsePort struct {
	messaging.Port
	checks int
}

func (p *retryResponsePort) CanSend() bool {
	p.checks++
	if p.checks == 2 {
		p.Port.RetrieveOutgoing()
	}
	return p.Port.CanSend()
}

func TestRespondStageDefersRetryUntilNextTick(t *testing.T) {
	simulation := sim.MakeBuilder().WithTaskTracking().Build()
	t.Cleanup(func() { require.NoError(t, simulation.Terminate()) })
	spec := DefaultSpec()
	spec.NumReqPerCycle = 4
	spec.Bottom.Destinations = []messaging.RemotePort{"Memory.Top"}
	ports := makePorts(simulation, "Cache", 1)
	port := &retryResponsePort{Port: ports.Top}
	ports.Top = port
	mw := buildStageTestCache(simulation, spec,
		Resources{Storage: mem.NewStorage(4 * mem.KB)}, ports, state{})
	mw.comp.State.Transactions = []transactionState{
		{Done: true, HasRead: true, ReadMeta: messaging.Msg{ID: 100, Src: "Source"}},
		{Done: true, HasWrite: true, WriteMeta: messaging.Msg{ID: 101, Src: "Source"}},
	}
	port.Send(messaging.Msg{ID: simulation.NewID(), Src: port.AsRemote(),
		Dst: "Source", Payload: memprotocol.WriteDoneRsp{}})
	before := simulation.NewID()
	require.False(t, mw.tickRespondStage())
	require.Equal(t, 1, port.checks)
	require.Equal(t, before+1, simulation.NewID())
	require.False(t, mw.comp.State.Transactions[0].Removed)
	// The second capacity check succeeds, but it must happen on a later tick.
	require.True(t, mw.tickRespondStage())
	require.Equal(t, 3, port.checks)
	require.True(t, mw.comp.State.Transactions[0].Removed)
	require.False(t, mw.comp.State.Transactions[1].Removed)
	rsp, ok := port.RetrieveOutgoing()
	require.True(t, ok)
	require.Equal(t, uint64(100), rsp.RspTo)
}

func TestRespondStagePreservesWidthAndOrder(t *testing.T) {
	simulation, mw := responseTestCache(2, 4)
	t.Cleanup(func() { require.NoError(t, simulation.Terminate()) })
	mw.comp.State.Transactions = []transactionState{
		{}, // An unfinished earlier request must not block completed requests.
		{Done: true, HasWrite: true, WriteMeta: messaging.Msg{ID: 100, Src: "Source"}},
		{Done: true, Removed: true},
		{Done: true, HasRead: true, ReadMeta: messaging.Msg{ID: 101, Src: "Source"}},
		{Done: true, HasWrite: true, WriteMeta: messaging.Msg{ID: 102, Src: "Source"}},
	}
	require.True(t, mw.tickRespondStage())
	for _, want := range []uint64{100, 101} {
		rsp, ok := mw.topPort().RetrieveOutgoing()
		require.True(t, ok)
		require.Equal(t, want, rsp.RspTo)
	}
	_, ok := mw.topPort().RetrieveOutgoing()
	require.False(t, ok)
	require.False(t, mw.comp.State.Transactions[4].Removed)
	require.True(t, mw.tickRespondStage())
	rsp, ok := mw.topPort().RetrieveOutgoing()
	require.True(t, ok)
	require.Equal(t, uint64(102), rsp.RspTo)
}

func BenchmarkRespondStageNoReadyTransactions(b *testing.B) {
	simulation, mw := responseTestCache(32, 32)
	defer simulation.Terminate()
	mw.comp.State.Transactions = make([]transactionState, 4096)
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		mw.tickRespondStage()
	}
}
