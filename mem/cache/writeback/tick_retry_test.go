package writeback

import (
	"testing"

	"github.com/sarchlab/akita/v5/mem"
	"github.com/sarchlab/akita/v5/mem/cache"
	"github.com/sarchlab/akita/v5/mem/memprotocol"
	akitasim "github.com/sarchlab/akita/v5/sim"
	"github.com/sarchlab/akita/v5/sim/messaging"
	"github.com/sarchlab/akita/v5/sim/tasks"
	"github.com/sarchlab/akita/v5/sim/timing"
	"github.com/stretchr/testify/require"
)

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

type retrySendPort struct {
	messaging.Port
	checks int
}

func (p *retrySendPort) CanSend() bool {
	p.checks++
	return p.Port.CanSend()
}

type retryTagRecorder struct {
	tasks.NopObserver
	tags []string
}

func (r *retryTagRecorder) TagAdded(tag tasks.Tag) { r.tags = append(r.tags, tag.What) }

func newRetryCache(t *testing.T) (*Comp, *retrySendPort) {
	t.Helper()
	sim := akitasim.MakeBuilder().WithTaskTracking().Build()
	t.Cleanup(func() { require.NoError(t, sim.Terminate()) })
	ports := makePorts(sim, "Cache", 1)
	top := &retrySendPort{Port: ports.Top}
	ports.Top = top
	comp := buildStageTestComp(sim, stageTestSpec(),
		Resources{Storage: mem.NewStorage(16 * mem.KB)}, ports)
	return comp, top
}

func TestBankFinalizeStopsAfterBlockedScanAndStillPulls(t *testing.T) {
	comp, top := newRetryCache(t)
	top.Port.Send(messaging.Msg{ID: comp.NewID(), Src: top.AsRemote(), Dst: "Source",
		Payload: memprotocol.WriteDoneRsp{}})
	s := &comp.State
	s.Transactions = []transactionState{
		{Action: bankReadHit}, {Action: bankReadHit}, {Action: bankWriteFetched},
	}
	s.BankPostPipelineBufs[0].Push(0)
	s.BankPostPipelineBufs[0].Push(1)
	s.WriteBufferToBankBufs[0].Push(2)

	require.True(t, comp.Middlewares.Pipeline.bankStages[0].Tick())

	require.Equal(t, 2, top.checks, "one scan checks each blocked entry once")
	require.Equal(t, 2, s.BankPostPipelineBufs[0].Size())
	require.Zero(t, s.WriteBufferToBankBufs[0].Size())
	require.Equal(t, 1, s.BankPipelines[0].Size())
}

func TestBankPullChecksInputsAndStopsAfterFailure(t *testing.T) {
	comp, _ := newRetryCache(t)
	s := &comp.State
	bank := comp.Middlewares.Pipeline.bankStages[0]
	pipeline := &s.BankPipelines[0]
	for pipeline.CanPush() {
		pipeline.Push(0)
	}
	clock := &retryClock{TimeTeller: comp}
	pipeline.BindClock(clock)

	require.False(t, bank.pullFromBuf())
	require.Zero(t, clock.reads, "no input means no capacity check")

	s.Transactions = []transactionState{{Action: bankReadHit}}
	s.DirToBankBufs[0].Push(0)
	reads := func(width int) int {
		comp.Spec.NumReqPerCycle = width
		clock.reads = 0
		require.False(t, bank.Tick())
		return clock.reads
	}
	require.Equal(t, reads(1), reads(64))
	require.Equal(t, 1, s.DirToBankBufs[0].Size())
}

func TestBankPullKeepsWriteBufferPriorityAndReservation(t *testing.T) {
	comp, _ := newRetryCache(t)
	s := &comp.State
	bank := comp.Middlewares.Pipeline.bankStages[0]
	s.BankDownwardInflightTransCounts[0] = bank.pipelineWidth - 1
	s.Transactions = []transactionState{{Action: bankWriteFetched}, {Action: bankReadHit}}
	s.WriteBufferToBankBufs[0].Push(0)
	s.DirToBankBufs[0].Push(1)

	require.True(t, bank.Tick())

	require.Zero(t, s.WriteBufferToBankBufs[0].Size())
	require.Equal(t, 1, s.DirToBankBufs[0].Size(), "the reserved lane stays closed to the directory")
	require.Equal(t, 1, s.BankPipelines[0].Size())
}

func TestDirectoryBlockedHeadTriesOncePerStep(t *testing.T) {
	comp, _ := newRetryCache(t)
	rec := &retryTagRecorder{}
	comp.TaskTracker().Subscribe(rec, comp)
	s := &comp.State
	spec := comp.Spec
	const addr = uint64(0x40)
	setID := cache.DirectorySetID(addr, 1<<spec.Log2BlockSize, spec.NumSets)
	block := &s.DirectoryState.Sets[setID].Blocks[0]
	block.IsValid, block.Tag, block.PID = true, addr, 1

	msg := messaging.Msg{ID: comp.NewID(), Src: "Source", Dst: comp.Ports.Top.AsRemote(),
		Payload: memprotocol.ReadReq{Address: addr, AccessByteSize: 4, PID: 1}}
	s.Transactions = []transactionState{{
		HasRead: true, ReadMeta: msg, ReadAddress: addr, ReadAccessByteSize: 4, ReadPID: 1,
		RecvTaskID: comp.TaskTracker().StartReqIn(comp, msg),
	}}
	s.DirPostPipelineBuf.Push(0)
	for s.DirToBankBufs[0].CanPush() {
		s.DirToBankBufs[0].Push(0)
	}

	mw := comp.Middlewares.Pipeline
	require.False(t, mw.runStage(mw.dirStage, spec.NumReqPerCycle))

	require.Equal(t, uint64(spec.NumReqPerCycle), s.DirStep, "every directory step still runs")
	require.Len(t, rec.tags, spec.NumReqPerCycle, "each step tries the blocked head once")
	require.Equal(t, 1, s.DirPostPipelineBuf.Size())
}
