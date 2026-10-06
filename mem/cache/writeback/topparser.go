package writeback

import (
	"github.com/sarchlab/akita/v5/mem/memprotocol"
	"github.com/sarchlab/akita/v5/sim/tracing"
)

type topParser struct {
	cache *pipelineMW
}

func (p *topParser) Tick() bool {
	next := &p.cache.comp.State

	if cacheState(next.CacheState) != cacheStateRunning {
		return false
	}

	msg, ok := p.cache.topPort().PeekIncoming()
	if !ok {
		return false
	}

	if !next.DirStageBuf.CanPush() {
		return false
	}

	trans := transactionState{
		ID: p.cache.comp.NewID(),
	}

	switch content := msg.Payload.(type) {
	case memprotocol.ReadReq:

		trans.HasRead = true
		trans.ReadMeta = msg
		trans.ReadAddress = content.Address
		trans.ReadAccessByteSize = content.AccessByteSize
		trans.ReadPID = content.PID
	case memprotocol.WriteReq:

		trans.HasWrite = true
		trans.WriteMeta = msg
		trans.WriteAddress = content.Address
		trans.WriteData = content.Data
		trans.WriteDirtyMask = content.DirtyMask
		trans.WritePID = content.PID
	}

	idx := next.allocTransaction(trans)
	next.DirStageBuf.Push(idx)

	// Admission milestone on the incoming-buffer task: the message left the Top
	// buffer because the directory stage buffer had room. The buffer task is
	// keyed by the peeked message and ends at RetrieveIncoming below.
	tracing.AddMilestone(p.cache.comp, tracing.Milestone{
		TaskID: tracing.MsgIDAtIncomingBuffer(msg, p.cache.comp),
		Kind:   tracing.MilestoneKindHardwareResource,
		What:   p.cache.comp.Name() + ".dir_stage_buf",
	})

	tracing.TraceReqReceive(p.cache.comp, msg)

	p.cache.topPort().RetrieveIncoming()

	return true
}
