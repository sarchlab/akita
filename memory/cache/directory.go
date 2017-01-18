package cache

// A Directory stores the information about what is stored in the cache.
//
// The directory can translate from the request address (can be either virtual
// addree or physical address) to the cache based address.
type Directory struct {
	NumSets   uint
	NumWays   uint
	BlockSize uint

	sets map[uint]*Set
}

// NewDirectory returns a new directory object
func NewDirectory() *Directory {
	d := new(Directory)
	d.sets = make(map[uint]*Set)
	return d
}

// TotalSize returns the maximum number of bytes can be stored in the cache
func (d *Directory) TotalSize() uint64 {
	return uint64(d.NumSets) * uint64(d.NumWays) * uint64(d.BlockSize)
}

// Get the set that a certain address should store at
func (d *Directory) getSet(reqAddr uint64) (set *Set, setID uint) {
	setID = uint(reqAddr / uint64(d.BlockSize) % uint64(d.NumSets))
	set, ok := d.sets[uint(setID)]
	if !ok {
		set = NewSet()
		d.sets[uint(setID)] = set
	}
	return
}

// Lookup finds the block that stores the reqAddr. If the reqAddr is valid
// in the cache, return the block information. Otherwise, return nil
func (d *Directory) Lookup(reqAddr uint64) *Block {
	set, _ := d.getSet(reqAddr)
	for _, block := range set.Blocks {
		if block.IsValid && block.Tag == reqAddr {
			return block
		}
	}
	return nil
}

// FindEmpty returns a block that can be used to stored the data at reqAddr.
func (d *Directory) FindEmpty(reqAddr uint64) *Block {
	set, setID := d.getSet(reqAddr)
	for _, block := range set.Blocks {
		if !block.IsValid {
			return block
		}
	}

	// The blocks in a set are allocated only after first time used. Therefore
	// we need to check if all the blocks has been used.
	if uint(len(set.Blocks)) < d.NumWays {
		block := new(Block)
		block.SetID = setID
		block.WayID = uint(len(set.Blocks))
		block.CacheAddress = uint64(block.SetID)*uint64(d.NumWays*d.BlockSize) +
			uint64(block.WayID)*uint64(d.BlockSize)
		set.Blocks = append(set.Blocks, block)
		return block
	}

	// No space available, need eviction
	return nil
}
