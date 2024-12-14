package tagging

// A VictimFinder decides with block should be evicted
type VictimFinder interface {
	FindVictim(tags Tags, address uint64) (Block, bool)
}

// LRUVictimFinder evicts the least recently used block to evict
type LRUVictimFinder struct {
}

// NewLRUVictimFinder returns a newly constructed lru evictor
func NewLRUVictimFinder() *LRUVictimFinder {
	e := new(LRUVictimFinder)
	return e
}

// FindVictim returns the least recently used block in a set
func (e *LRUVictimFinder) FindVictim(tags Tags, address uint64) (Block, bool) {
	set, _ := tags.GetSet(address)

	// First try evicting an empty block
	for _, blockIndex := range set.LRUQueue {
		block := set.Blocks[blockIndex]

		if !block.IsValid && !block.IsLocked {
			return block, true
		}
	}

	for _, blockIndex := range set.LRUQueue {
		block := set.Blocks[blockIndex]
		if !block.IsLocked {
			return block, true
		}
	}

	return Block{}, false
}
