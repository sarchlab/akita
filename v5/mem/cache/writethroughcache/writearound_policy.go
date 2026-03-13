package writethroughcache

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
	postCoalesceIdx int,
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

	if !trans.HasWriteToBottom {
		ok := d.writeBottom(trans)
		if !ok {
			return false
		}
	}

	addr := trans.WriteAddress
	spec := d.cache.GetSpec()
	blockSize := uint64(1 << spec.Log2BlockSize)
	cacheLineID := addr / blockSize * blockSize

	nextBlock := &next.DirectoryState.Sets[setID].Blocks[wayID]
	nextBlock.IsLocked = true
	nextBlock.IsValid = true
	nextBlock.Tag = cacheLineID
	cache.DirectoryVisit(&next.DirectoryState, setID, wayID)

	trans.BankAction = bankActionWrite
	trans.BlockSetID = setID
	trans.BlockWayID = wayID
	trans.HasBlock = true

	bankBuf.PushTyped(postCoalesceIdx)

	tracing.AddTaskStep(trans.ID, d.cache.comp, "write-hit")

	dirPostBuf := &next.DirPostBuf
	dirPostBuf.Elements = dirPostBuf.Elements[1:]

	return true
}

// HandleWriteMiss handles a write miss for write-around policy.
func (p *WritearoundPolicy) HandleWriteMiss(
	d *directory,
	trans *transactionState,
	postCoalesceIdx int,
) bool {
	if ok := d.writeBottom(trans); ok {
		tracing.AddTaskStep(trans.ID, d.cache.comp, "write-miss")

		next := d.cache.comp.GetNextState()
		dirPostBuf := &next.DirPostBuf
		dirPostBuf.Elements = dirPostBuf.Elements[1:]

		return true
	}

	return false
}

// NeedsDualCompletion returns true for write-around policy since both the
// bank write and the bottom write must complete before the transaction is done.
func (p *WritearoundPolicy) NeedsDualCompletion() bool {
	return true
}
