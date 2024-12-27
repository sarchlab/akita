package tagging

import (
	"github.com/sarchlab/akita/v4/mem/vm"
)

type TagArray interface {
	Lookup(PID vm.PID, reqAddr uint64) (Block, bool)
	Update(block Block)
	Visit(block Block)
	GetSet(reqAddr uint64) (set *Set, setID int)
	Lock(setID, wayID int)
	Unlock(setID, wayID int)
	Reset()
}

func NewTagArray(
	numSets int,
	numWays int,
	blockSize int,
) TagArray {
	t := &tagArrayImpl{
		NumSets:   numSets,
		NumWays:   numWays,
		BlockSize: blockSize,
		Sets:      []Set{},
	}

	t.Reset()

	return t
}

// A Block of a cache is the information that is associated with a cache line
type Block struct {
	PID          vm.PID
	Tag          uint64
	WayID        int
	SetID        int
	CacheAddress uint64
	IsValid      bool
	IsDirty      bool
	ReadCount    int
	IsLocked     bool
	DirtyMask    []bool
}

// A Set is a list of blocks where a certain piece memory can be stored at.
type Set struct {
	Blocks   []Block
	LRUQueue []int
}

type tagArrayImpl struct {
	NumSets   int
	NumWays   int
	BlockSize int
	Sets      []Set
}

// TotalSize returns the maximum number of bytes can be stored in the cache
func (d *tagArrayImpl) TotalSize() uint64 {
	return uint64(d.NumSets) * uint64(d.NumWays) * uint64(d.BlockSize)
}

// Get the set that a certain address should store at
func (d *tagArrayImpl) GetSet(reqAddr uint64) (set *Set, setID int) {
	setID = int(reqAddr / uint64(d.BlockSize) % uint64(d.NumSets))
	set = &d.Sets[setID]

	return
}

// Lookup finds the block that reqAddr. If the reqAddr is valid
// in the cache, return the block information. Otherwise, return nil
func (d *tagArrayImpl) Lookup(PID vm.PID, reqAddr uint64) (Block, bool) {
	set, _ := d.GetSet(reqAddr)
	for _, block := range set.Blocks {
		if block.IsValid && block.Tag == reqAddr && block.PID == PID {
			return block, true
		}
	}

	return Block{}, false
}

// Update updates the block information
func (d *tagArrayImpl) Update(block Block) {
	d.Sets[block.SetID].Blocks[block.WayID] = block
}

// Visit moves the block to the end of the LRUQueue
func (d *tagArrayImpl) Visit(block Block) {
	set := &d.Sets[block.SetID]
	newLRUQueue := []int{}

	for _, b := range set.LRUQueue {
		if b != block.WayID {
			newLRUQueue = append(newLRUQueue, b)
		}
	}

	newLRUQueue = append(newLRUQueue, block.WayID)

	set.LRUQueue = newLRUQueue
}

// Reset will mark all the blocks in the directory invalid
func (d *tagArrayImpl) Reset() {
	d.Sets = make([]Set, d.NumSets)
	for i := 0; i < d.NumSets; i++ {
		for j := 0; j < d.NumWays; j++ {
			block := Block{
				IsValid: false,
				SetID:   i,
				WayID:   j,
			}

			d.Sets[i].Blocks = append(d.Sets[i].Blocks, block)
			d.Sets[i].LRUQueue = append(d.Sets[i].LRUQueue, j)
		}
	}
}

func (d *tagArrayImpl) Lock(setID, wayID int) {
	d.Sets[setID].Blocks[wayID].IsLocked = true
}

func (d *tagArrayImpl) Unlock(setID, wayID int) {
	d.Sets[setID].Blocks[wayID].IsLocked = false
}
