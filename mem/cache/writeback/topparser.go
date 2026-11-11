package writeback

import (
	"github.com/sarchlab/akita/v5/mem/memprotocol"
	"github.com/sarchlab/akita/v5/sim/tasks"
)

type topParser struct {
	cache *pipelineMW
}

func (p *topParser) Tick() bool {
	next := &p.cache.comp.State

	if cacheState(next.CacheState) != cacheStateRunning {
		return false
	}

	if next.Maintenance.Resume(&next.DirStageBuf) {
		return true
	}

	return next.Maintenance.Admit(p.acceptTop, p.issueMaintenanceLookup)
}

func (p *topParser) acceptTop() bool {
	next := &p.cache.comp.State
	msg, ok := p.cache.topPort().PeekIncoming()
	if !ok {
		return false
	}

	blocked := next.Maintenance.Blocks(msg, p.cache.comp.Spec.Log2BlockSize)
	if blocked && len(next.Maintenance.Deferred) >= p.cache.comp.Spec.NumReqPerCycle ||
		!blocked && !next.DirStageBuf.CanPush() {
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
	if blocked {
		next.Maintenance.Deferred = append(next.Maintenance.Deferred, idx)
	} else {
		next.DirStageBuf.Push(idx)
	}

	// Admission milestone on the incoming-buffer task: the message left the Top
	// buffer because the directory stage buffer had room. The buffer task is
	// owned by the port head and ends at RetrieveIncoming below.
	p.cache.topPort().AddIncomingMilestone(
		tasks.Reason(tasks.MilestoneKindHardwareResource, p.cache.comp.Name()+".dir_stage_buf"))

	next.Transactions[idx].RecvTaskID = p.cache.comp.TaskTracker().StartReqIn(p.cache.comp, msg)

	p.cache.topPort().RetrieveIncoming()

	return true
}
