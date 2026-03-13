// WritePolicy defines the strategy for handling write operations in the cache.
package simplecache

// WritePolicy defines the strategy for handling write operations in the cache.
type WritePolicy interface {
	// HandleWriteHit handles a write hit in the directory.
	// Returns true if progress was made.
	HandleWriteHit(d *directory, trans *transactionState,
		setID, wayID int, postCoalesceIdx int) bool

	// HandleWriteMiss handles a write miss in the directory.
	// Returns true if progress was made.
	HandleWriteMiss(d *directory, trans *transactionState,
		postCoalesceIdx int) bool

	// NeedsDualCompletion returns true if write-hit transactions need
	// both bank and bottom-write completion before cleanup.
	NeedsDualCompletion() bool
}
