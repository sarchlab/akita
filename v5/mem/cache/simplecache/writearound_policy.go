package simplecache

import (
	"github.com/sarchlab/akita/v5/mem/cache"
	"github.com/sarchlab/akita/v5/tracing"
)

// WritearoundPolicy implements WritePolicy for write-around caches.
// On a write hit, it updates the cache block and writes through to the
// bottom. Both bank and bottom writes must complete (dual completion).
// On a write miss, it writes directly to the bottom without caching.
type WritearoundPolicy struct{}

// HandleWriteHit handles a write hit for write-around policy.
func (p *WritearoundPolicy) HandleWriteHit(
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

	tracing.AddTaskStep(trans.id, d.cache.comp, "write-hit")

	dirPostBuf := &next.DirPostBuf
	dirPostBuf.Pop()

	return true
}

// HandleWriteMiss handles a write miss for write-around policy.
func (p *WritearoundPolicy) HandleWriteMiss(
	d *directory,
	trans *transactionState,
) bool {
	if ok := d.writeBottom(trans); ok {
		tracing.AddTaskStep(trans.id, d.cache.comp, "write-miss")

		next := d.cache.comp.GetNextState()
		dirPostBuf := &next.DirPostBuf
		dirPostBuf.Pop()

		return true
	}

	return false
}

// NeedsDualCompletion returns true for write-around policy since both the
// bank write and the bottom write must complete before the transaction is done.
func (p *WritearoundPolicy) NeedsDualCompletion() bool {
	return true
}
