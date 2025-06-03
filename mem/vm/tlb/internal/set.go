// Package internal provides the definition required for defining TLB.
package internal

import (
	"fmt"
	"sort"

	"github.com/sarchlab/akita/v3/mem/vm"
	// "github.com/sarchlab/akita/v3/mem/vm/tlb"
)

// A Set holds a certain number of Pages. Lookup,update,Evict and Visit are the operations which we can perform on set
type Set interface {
	Lookup(pid vm.PID, vAddr uint64) (wayID int, Page vm.Page, found bool)
	Update(wayID int, Page vm.Page)
	Evict() (wayID int, ok bool)
	Visit(wayID int)
	//PACLookup(pid vm.PID, vAddr uint64) (block *block, found bool)
	AddEntry(Page vm.Page, readWrite bool, FaultCount uint64)
}

// NewSet creates a new TLB set.
func NewSet(numWays int) Set { //numWays int specifies the number of ways (or blocks) in the cache set. This function creates a new Set object

	s := &SetImpl{} //Allocates memory for this struct and initializes its fields with zero values.
	s.blocks = make([]*block, numWays)
	s.visitList = make([]*block, 0, numWays)
	s.vAddrWayIDMap = make(map[string]int)
	for i := range s.blocks {
		b := &block{}
		s.blocks[i] = b
		b.wayID = i
		s.Visit(i)
		//b.FaultCount = 0
	}
	return s
}

type block struct { // block is same as way. one way stores information about Page,wayId and lastVisit (visit for LRU eviction)
	Page      vm.Page
	wayID     int
	lastVisit uint64

	ReadWrite  bool // false for read, true for write
	FaultCount uint8
	//Modified   bool // Tracks if an entry has been modified before eviction //
}

func (b *block) Less(anotherBlock *block) bool { //use to compare time for LRU evictions
	return b.lastVisit < anotherBlock.lastVisit
} /*

Receiver (b *block):
This makes 'Less' a method of the 'block' type. The receiver 'b' refers to the current instance of 'block' on which this method is called.
anotherBlock *block :
The method takes a pointer to 'another block' instance (anotherBlock) as an argument for comparison.
b.lastVisit < anotherBlock.lastVisit:
It checks whether the 'lastVisit' value of the current 'block' is less than the 'lastVisit' value of the 'other block'.
*/

type SetImpl struct {
	blocks        []*block       //A slice (dynamic array) of pointers to block objects. Represents the blocks (or cache lines) in the set, one for each way.
	vAddrWayIDMap map[string]int // A map (Go's dictionary or hash table) with keys of type string and values of type int.stores a mapping of virtual addresses (as string) to their corresponding way ID (an integer identifier within a set).
	//This field helps quickly look up the location (way ID) of a specific block using its virtual address.
	visitList  []*block // slice of pointers to block objects.
	visitCount uint64   //A counter to track the number of block visits or accesses.
}

func (s *SetImpl) GetBlocks() []*block {
	return s.blocks
}

func (s *SetImpl) keyString(pid vm.PID, vAddr uint64) string {
	return fmt.Sprintf("%d%016x", pid, vAddr) //"%d": Formats the pid as a decimal integer. "%016x": Formats the vAddr as a hexadecimal value, padded with zeros to be 16 characters wide.
}

/*
Receiver (s *SetImpl):
    The method belongs to the 'SetImpl' type. 's' is the receiver instance of 'SetImpl'.
Return Type (string):
    The method returns a string created from pid and vAddr
Returns : The pid (decimal) and vAddr (hexadecimal) are concatenated into a single string to give unique id.
*/

func (s *SetImpl) Lookup(pid vm.PID, vAddr uint64) (
	wayID int,
	Page vm.Page,
	found bool, //wayId,Page and found is return values of this function
) { //This method searches for a block based on the pid and vAddr, using a map (vAddrWayIDMap) to look up the block's location (wayID).
	key := s.keyString(pid, vAddr)
	wayID, ok := s.vAddrWayIDMap[key]
	if !ok {
		return 0, vm.Page{}, false
	}

	block := s.blocks[wayID]

	return block.wayID, block.Page, true
}

// func (s *SetImpl) PACLookup(pid vm.PID, vAddr uint64) (
// 	block *block,
// 	found bool, //wayId,Page and found is return values of this function
// ) { //This method searches for a block based on the pid and vAddr, using a map (vAddrWayIDMap) to look up the block's location (wayID).
// 	key := s.keyString(pid, vAddr)

// 	fmt.Printf("[PRINT VMAP]-----\n")
// 	fmt.Println(s.vAddrWayIDMap)
// 	fmt.Printf("[PRINT VMAP]-----\n")
// 	wayID, ok := s.vAddrWayIDMap[key]
// 	if !ok {
// 		return block, false
// 	}

// 	block = s.blocks[wayID]

// 	return block, true
// }

// Implementation

func (s *SetImpl) AddEntry(Page vm.Page, readWrite bool, FaultCount uint64) {
	// Check if the Page is already in cache
	if wayID, exists := s.vAddrWayIDMap[s.keyString(Page.PID, Page.VAddr)]; exists {
		s.Visit(wayID) // Update LRU order
		return
	}

	// Find an empty block
	for _, blk := range s.blocks {
		if blk.Page.VAddr == 0 { // Empty block found
			blk.Page = Page
			blk.ReadWrite = readWrite
			blk.FaultCount = uint8(FaultCount)
			s.Visit(blk.wayID) // Update LRU
			s.vAddrWayIDMap[s.keyString(Page.PID, Page.VAddr)] = blk.wayID
			return
		}
	}

	// No empty block, evict the LRU block
	if s.hasNothingToEvict() {
		return // Nothing to evict, so return
	}

	fmt.Printf("from Add entry\n")
	wayID, ok := s.Evict()

	fmt.Printf("Way ID: %d", wayID)

	if !ok {
		return
	}

	s.AddEntry(Page, readWrite, FaultCount)

	// lruBlock := s.visitList[0] // Least Recently Used block

	// Move evicted block to PA Table
	//vpn := lruBlock.Page.VAddr // Assuming VPN is derived from Virtual Address
	//PA_Table.UpdateEntry(vpn, lruBlock.ReadWrite, true)

	// Remove old mapping from cache
	// delete(s.vAddrWayIDMap, s.keyString(lruBlock.Page.PID, lruBlock.Page.VAddr))
	// fmt.Printf("Eviction happen :Remove %x and Add %x \n", lruBlock.Page.VAddr, Page.VAddr)
	// // Replace LRU block with new entry
	// lruBlock.Page = Page
	// lruBlock.ReadWrite = readWrite
	// lruBlock.FaultCount = uint8(FaultCount)
	// s.Visit(lruBlock.wayID) // Update LRU
	// s.vAddrWayIDMap[s.keyString(Page.PID, Page.VAddr)] = lruBlock.wayID
	// s.blocks[lruBlock.wayID] = lruBlock

}

func (s *SetImpl) Update(wayID int, Page vm.Page) { //The Update method modifies an existing block in the set by updating its Page and also updates the vAddrWayIDMap to reflect the change. It first removes the old mapping and then adds the new one, ensuring the map remains accurate.
	block := s.blocks[wayID]                             /// Retrieve the block at the specified wayID in the blocks array.
	key := s.keyString(block.Page.PID, block.Page.VAddr) // Retrieve the block at the specified wayID in the blocks array.
	delete(s.vAddrWayIDMap, key)                         //delete old entry

	block.Page = Page //add new Page entry
	key = s.keyString(Page.PID, Page.VAddr)
	s.vAddrWayIDMap[key] = wayID // Add the new mapping of the virtual address to the `wayID` in the vAddrWayIDMap
}

func (s *SetImpl) Evict() (wayID int, ok bool) { //It removes the least recently used block from the set and returns the wayID of the evicted block. If there is nothing to evict, it returns false
	if s.hasNothingToEvict() {
		return 0, false
	}

	//Original - wayID = s.visitTree.DeleteMin().(*block).wayID
	leastVisited := s.visitList[0]
	wayID = leastVisited.wayID
	s.visitList = s.visitList[1:] //This operation updates s.visitList by setting it to the new slice created by visitList[1:], which excludes the first element (visitList[0]).
	return wayID, true
}

func (s *SetImpl) Visit(wayID int) { // This function will update visitlist for LRU eviction
	block := s.blocks[wayID] //get a block from blocks

	for i, b := range s.visitList { // purpose of loop - if the block already exists in the visitList (indicating it was accessed before),
		//remove it to avoid duplicates before reinserting it as the most recently accessed.
		if b.wayID == wayID { //if found matching block i.e visted before
			s.visitList = append(s.visitList[:i], s.visitList[i+1:]...) // Remove block - Use slicing to create a new visitList without this block.
		}
	}

	s.visitCount++                 // This acts as a timestamp for the most recent access.
	block.lastVisit = s.visitCount // helps track when the block was last accessed

	//Determine the new position of the block in the visitList
	index := sort.Search(len(s.visitList), func(i int) bool {
		return s.visitList[i].lastVisit > block.lastVisit
	})

	/*
			sort.Search: Uses binary search to find the correct position in the visitList where the block should be inserted, maintaining the list sorted by lastVisit.
		    Lambda Function (func(i int) bool): Compares each block in the list to find the first block with a lastVisit value greater than the current block’s lastVisit.
	*/
	s.visitList = append(s.visitList, nil)           //Expands the visitList by one to make room for the new block.
	copy(s.visitList[index+1:], s.visitList[index:]) //Shifts elements in the list to the right, starting from the index determined earlier.
	s.visitList[index] = block                       // put block in index position
}

func (s *SetImpl) hasNothingToEvict() bool { //check length
	return len(s.visitList) == 0
}
