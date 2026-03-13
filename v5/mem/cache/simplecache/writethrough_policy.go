package simplecache

import (
	"github.com/sarchlab/akita/v5/mem/cache"
	"github.com/sarchlab/akita/v5/mem/mem"
	"github.com/sarchlab/akita/v5/tracing"
)

// WritethroughPolicy implements WritePolicy for write-through caches.
// On a write hit, it updates the cache block and writes through to the
// bottom. Both bank and bottom writes must complete (dual completion).
// On a write miss, partial writes fetch the line first; full-line writes
// allocate a victim and treat it as a write hit.
type WritethroughPolicy struct{}

// HandleWriteHit handles a write hit for write-through policy.
// This is identical to the write-around write-hit behavior.
func (p *WritethroughPolicy) HandleWriteHit(
	d *directory,
	trans *transactionState,
	setID, wayID int,
) bool {
	next := d.cache.comp.GetNextState()
	block := &next.DirectoryState.Sets[setID].Blocks[wayID]
	if block.IsLocked || block.ReadCount > 0 {
		return false
	}

	bankBuf := d.getBankBuf(setID, wayID)
	if !bankBuf.CanPush() {
		return false
	}

	if trans.writeToBottom == nil {
		ok := d.writeBottom(trans)
		if !ok {
			return false
		}
	}

	addr := trans.write.Address
	spec := d.cache.GetSpec()
	blockSize := uint64(1 << spec.Log2BlockSize)
	cacheLineID := addr / blockSize * blockSize

	nextBlock := &next.DirectoryState.Sets[setID].Blocks[wayID]
	nextBlock.IsLocked = true
	nextBlock.IsValid = true
	nextBlock.Tag = cacheLineID
	cache.DirectoryVisit(&next.DirectoryState, setID, wayID)

	trans.bankAction = bankActionWrite
	trans.blockSetID = setID
	trans.blockWayID = wayID
	trans.hasBlock = true

	transIdx := d.findPostCoalesceTransIdx(trans)
	bankBuf.PushTyped(transIdx)

	dirPostBuf := &next.DirPostBuf
	dirPostBuf.Pop()

	return true
}

// HandleWriteMiss handles a write miss for write-through policy.
// Partial writes use MSHR fetch + write; full-line writes allocate a victim
// and process as a write hit.
func (p *WritethroughPolicy) HandleWriteMiss(
	d *directory,
	trans *transactionState,
) bool {
	if p.isPartialWrite(d, trans.write) {
		return p.partialWriteMiss(d, trans)
	}

	ok := p.fullLineWriteMiss(d, trans)
	if ok {
		tracing.AddTaskStep(trans.id, d.cache.comp, "write-miss")
	}

	return ok
}

// NeedsDualCompletion returns true for write-through policy since both the
// bank write and the bottom write must complete before the transaction is done.
func (p *WritethroughPolicy) NeedsDualCompletion() bool {
	return true
}

func (p *WritethroughPolicy) isPartialWrite(
	d *directory,
	writeMsg *mem.WriteReq,
) bool {
	spec := d.cache.GetSpec()
	if len(writeMsg.Data) < (1 << spec.Log2BlockSize) {
		return true
	}

	if writeMsg.DirtyMask != nil {
		for _, byteDirty := range writeMsg.DirtyMask {
			if !byteDirty {
				return true
			}
		}
	}

	return false
}

func (p *WritethroughPolicy) partialWriteMiss(
	d *directory,
	trans *transactionState,
) bool {
	addr := trans.write.Address
	spec := d.cache.GetSpec()
	blockSize := uint64(1 << spec.Log2BlockSize)
	cacheLineID := addr / blockSize * blockSize
	trans.fetchAndWrite = true

	next := d.cache.comp.GetNextState()

	if cache.MSHRIsFull(&next.MSHRState, spec.NumMSHREntry) {
		return false
	}

	victimSetID, victimWayID := cache.DirectoryFindVictim(
		&next.DirectoryState, spec.NumSets, int(blockSize), cacheLineID)
	victim := &next.DirectoryState.Sets[victimSetID].Blocks[victimWayID]
	if victim.ReadCount > 0 || victim.IsLocked {
		return false
	}

	sentThisCycle := false

	if trans.writeToBottom == nil {
		ok := d.writeBottom(trans)
		if !ok {
			return false
		}

		sentThisCycle = true
	}

	ok := d.fetchFromBottom(trans, victimSetID, victimWayID)
	if !ok {
		return sentThisCycle
	}

	dirPostBuf := &next.DirPostBuf
	dirPostBuf.Pop()
	tracing.AddTaskStep(trans.id, d.cache.comp, "write-miss")

	return true
}

func (p *WritethroughPolicy) fullLineWriteMiss(
	d *directory,
	trans *transactionState,
) bool {
	addr := trans.write.Address
	spec := d.cache.GetSpec()
	blockSize := uint64(1 << spec.Log2BlockSize)
	cacheLineID := addr / blockSize * blockSize
	next := d.cache.comp.GetNextState()

	victimSetID, victimWayID := cache.DirectoryFindVictim(
		&next.DirectoryState, spec.NumSets, int(blockSize), cacheLineID)

	_ = next // suppress unused warning

	return p.HandleWriteHit(d, trans, victimSetID, victimWayID)
}
