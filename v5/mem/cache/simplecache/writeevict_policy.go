package simplecache

import (
	"github.com/sarchlab/akita/v5/tracing"
)

// WriteevictPolicy implements WritePolicy for write-evict caches.
// On a write hit, it invalidates the cache block and writes to the bottom.
// There is no bank processing for write hits (no dual completion).
// On a write miss, it writes directly to the bottom without caching.
type WriteevictPolicy struct{}

// HandleWriteHit handles a write hit for write-evict policy.
func (p *WriteevictPolicy) HandleWriteHit(
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

	nextBlock := &next.DirectoryState.Sets[setID].Blocks[wayID]
	nextBlock.IsValid = false

	tracing.AddTaskStep(trans.id, d.cache.comp, "write-hit")

	dirPostBuf := &next.DirPostBuf
	dirPostBuf.Pop()

	return true
}

// HandleWriteMiss handles a write miss for write-evict policy.
func (p *WriteevictPolicy) HandleWriteMiss(
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

// NeedsDualCompletion returns false for write-evict policy since there
// is no bank processing for write hits.
func (p *WriteevictPolicy) NeedsDualCompletion() bool {
	return false
}
