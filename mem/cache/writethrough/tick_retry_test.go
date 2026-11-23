package writethrough

import (
	"testing"

	"github.com/sarchlab/akita/v5/mem"
	"github.com/sarchlab/akita/v5/mem/cache/internal/maintenance"
	"github.com/sarchlab/akita/v5/mem/memcontrolprotocol"
	"github.com/sarchlab/akita/v5/mem/memprotocol"
	"github.com/sarchlab/akita/v5/sim"
	"github.com/sarchlab/akita/v5/sim/messaging"
	"github.com/sarchlab/akita/v5/sim/queueing"
	"github.com/sarchlab/akita/v5/sim/timing"
	"github.com/stretchr/testify/require"
)

type retryPeekPort struct {
	messaging.Port
	peeks int
}

func (p *retryPeekPort) PeekIncoming() (messaging.Msg, bool) {
	p.peeks++
	return p.Port.PeekIncoming()
}

// retryClock counts clock reads, which a non-empty pipeline makes on every
// capacity check.
type retryClock struct {
	timing.TimeTeller
	reads int
}

func (c *retryClock) CurrentTime() timing.VTimeInPicoSec {
	c.reads++
	return c.TimeTeller.CurrentTime()
}

func newRetryCache(t *testing.T) (*pipelineMW, *retryPeekPort, *retryPeekPort) {
	t.Helper()
	simulation := sim.MakeBuilder().WithTaskTracking().Build()
	t.Cleanup(func() { require.NoError(t, simulation.Terminate()) })
	spec := DefaultSpec()
	spec.NumBanks = 1
	spec.NumReqPerCycle = 4
	spec.Bottom.Destinations = []messaging.RemotePort{"Memory.Top"}
	ports := makePorts(simulation, "Cache", 4)
	top := &retryPeekPort{Port: ports.Top}
	bottom := &retryPeekPort{Port: ports.Bottom}
	ports.Top, ports.Bottom = top, bottom
	mw := buildStageTestCache(simulation, spec,
		Resources{Storage: mem.NewStorage(4 * mem.KB)}, ports, state{
			DirBuf:        queueing.MakeBuffer[int](1),
			BankBufs:      []queueing.Buffer[int]{queueing.MakeBuffer[int](1)},
			BankPostBufs:  []queueing.Buffer[int]{queueing.MakeBuffer[int](1)},
			BankPipelines: []queueing.FastPipeline[int]{stageBankPipeline(1, 2)},
		})
	return mw, top, bottom
}

func TestBottomParserStopsAtBlockedFill(t *testing.T) {
	mw, _, bottom := newRetryCache(t)
	s := &mw.comp.State
	s.BankBufs[0].Push(0)
	s.Transactions = []transactionState{{
		HasReadToBottom: true, ReadToBottomMeta: messaging.Msg{ID: 9},
	}}
	bottom.Deliver(messaging.Msg{ID: mw.comp.NewID(), RspTo: 9,
		Payload: memprotocol.DataReadyRsp{Data: make([]byte, 64)}})

	require.False(t, mw.tickParseBottomStage())

	require.Equal(t, 1, bottom.peeks)
	_, ok := bottom.Port.PeekIncoming()
	require.True(t, ok)
}

func TestBottomParserKeepsDrainingStaleResponses(t *testing.T) {
	mw, _, bottom := newRetryCache(t)
	for range 2 {
		bottom.Deliver(messaging.Msg{ID: mw.comp.NewID(), RspTo: 99,
			Payload: memprotocol.WriteDoneRsp{}})
	}

	require.True(t, mw.tickParseBottomStage())

	require.Equal(t, 3, bottom.peeks, "two responses, then the empty queue")
	_, ok := bottom.Port.PeekIncoming()
	require.False(t, ok)
}

func TestIntakeStopsAfterBlockedAdmission(t *testing.T) {
	for _, lastIssuedLookup := range []bool{false, true} {
		mw, top, _ := newRetryCache(t)
		s := &mw.comp.State
		s.DirBuf.Push(0)
		s.Transactions = []transactionState{{Removed: true}}
		s.Maintenance = maintenance.State{
			Command: memcontrolprotocol.CmdInvalidate, AccessesDrained: true,
			Lines: []uint64{0x40}, LastIssuedLookup: lastIssuedLookup,
		}
		top.Deliver(messaging.Msg{ID: mw.comp.NewID(), Src: "Source",
			Payload: memprotocol.ReadReq{Address: 0x1000, AccessByteSize: 4}})

		require.False(t, mw.tickIntakeStage())

		require.Equal(t, 1, top.peeks)
		require.True(t, s.WaitingForDir, "the first attempt records the admission wait")
		require.Equal(t, []uint64{0x40}, s.Maintenance.Lines)
		require.Equal(t, lastIssuedLookup, s.Maintenance.LastIssuedLookup)
	}
}

func TestBankExtractStopsAfterBlockedPipeline(t *testing.T) {
	mw, _, _ := newRetryCache(t)
	s := &mw.comp.State
	s.Transactions = []transactionState{{}}
	s.BankPipelines[0].Push(0)
	s.BankBufs[0].Push(0)
	clock := &retryClock{TimeTeller: mw.comp}
	s.BankPipelines[0].BindClock(clock)
	bank := mw.bankStages[0]
	reads := func(width int) int {
		bank.numReqPerCycle = width
		clock.reads = 0
		require.False(t, bank.Tick())
		return clock.reads
	}

	require.Equal(t, reads(1), reads(64))
	require.Equal(t, 1, s.BankBufs[0].Size())
}
