package vm

import (
	"fmt"
	"sort"
)

// PAEntry represents an entry in the PA-Table
type PAEntry struct {
	VPN        uint64
	ReadWrite  bool // false for read, true for write
	FaultCount uint8
	Modified   bool // Tracks if an entry has been modified before eviction
}

// PA-Table (Direct-Mapped Cache)
type PATable struct {
	Entries   []PAEntry
	TableSize int
}

func NewPATable(tableSize int) *PATable {
	return &PATable{
		Entries:   make([]PAEntry, tableSize),
		TableSize: tableSize,
	}
}

func (pa *PATable) GetEntry(vpn uint64) (*PAEntry, bool) {
	index := vpn % uint64(pa.TableSize)
	entry := &pa.Entries[index]
	if entry.VPN == vpn {
		return entry, true
	}
	return nil, false
}

func (pa *PATable) DeleteEntry(vpn uint64) {
	index := vpn % uint64(pa.TableSize)
	entry := &pa.Entries[index]

	if entry.VPN == vpn {
		*entry = PAEntry{} // Reset to default empty state

	} else {
		fmt.Printf("No entries in PATable")
	}

}

func (cache *PACache) RemoveEntry(setIndex uint64, VPT uint64) {
	for i, entry := range cache.Sets[setIndex] {
		if entry.VPT == VPT {
			cache.Sets[setIndex] = append(cache.Sets[setIndex][:i], cache.Sets[setIndex][i+1:]...)
			break
		}
	}
}

func (pa *PATable) AddEntry(vpn uint64, readWrite bool, faultIncrement bool) {
	index := vpn % uint64(pa.TableSize)
	entry := &pa.Entries[index]

	*entry = PAEntry{VPN: vpn, ReadWrite: readWrite, FaultCount: 1, Modified: true}
}

func (pa *PATable) UpdateEntry(vpn uint64, readWrite bool, faultIncrement bool) {
	index := vpn % uint64(pa.TableSize)
	entry := &pa.Entries[index]

	if entry.VPN == vpn {
		if faultIncrement {
			entry.FaultCount++
		}
		if !entry.ReadWrite && readWrite {
			entry.ReadWrite = true // Once set to write, it remains write
		}
		entry.Modified = true
	} else {
		// Replace existing entry in the direct-mapped slot
		*entry = PAEntry{VPN: vpn, ReadWrite: readWrite, FaultCount: 1, Modified: true}
	}
}

type PACEntry struct {
	VPT        uint64
	ReadWrite  bool // false for read, true for write
	FaultCount uint8
	Modified   bool // Tracks if an entry has been modified before eviction
	LastVisit  uint64
}

// PA-Cache (Write-Allocate + Write-Back Cache)
type PACache struct {
	Sets       map[uint64][]*PACEntry
	VisitList  map[uint64][]*PACEntry
	CacheSize  int
	WaySize    int
	FaultLimit uint8
	VisitCount uint64
}

func NewPACache(cacheSize, waySize int) *PACache {
	return &PACache{
		Sets:       make(map[uint64][]*PACEntry),
		VisitList:  make(map[uint64][]*PACEntry),
		CacheSize:  cacheSize,
		WaySize:    waySize,
		FaultLimit: 3, // Example threshold
	}
}

// GetSetIndex computes the set index from VPN
func (cache *PACache) GetSetIndex(vpn uint64) uint64 {
	return (vpn >> 12) & 0xF // Extract 4-bit set index
}

// GetVPT extracts the Virtual Page Tag (VPT)
func (cache *PACache) GetVPT(vpn uint64) uint64 {
	return vpn >> 16 // Extract VPT after removing index bits
}

// GetVPT extracts the Virtual Page Tag (VPT)
func (cache *PACache) GetVPN(vpn uint64) uint64 {
	return vpn >> 12 // Extract VPT after removing index bits
}
func (cache *PACache) HandlePageFault(vAddr uint64, readWrite bool, PA_Table *PATable) {
	setIndex := cache.GetSetIndex(vAddr)
	vpt := cache.GetVPT(vAddr)
	vpn := cache.GetVPN(vAddr)
	// Check if entry exists in PA-Cache
	for _, entry := range cache.Sets[setIndex] {
		if entry.VPT == vpt {
			// Cache Hit: Update fault counter and read/write bit
			entry.FaultCount++
			if !entry.ReadWrite && readWrite {
				entry.ReadWrite = true
			}
			cache.Visit(setIndex, entry)

			fmt.Println("Cache Hit: Updated entry in PA-Cache")
			// If fault counter reaches threshold remove from both PA-Cache and put into PA Table
			if entry.FaultCount >= cache.FaultLimit {
				fmt.Println("Fault threshold reached, deleting entry from PA-Cache and PA-Table")
				PA_Table.DeleteEntry(vpn) //delete PATable entry

				var newSet []*PACEntry
				for _, entry1 := range cache.Sets[setIndex] {
					if entry1.VPT != vpt {
						newSet = append(newSet, entry1)
					}
				}
				cache.Sets[setIndex] = newSet
				fmt.Println("\nnew scheme")
			}
			return
		}

	}
	// Cache Miss: Fetch from PA-Table (Write-Allocate)
	fmt.Println("Cache Miss: Fetching from PA-Table")

	paEntry, exists := PA_Table.GetEntry(vpn)
	if !exists {
		// Not in PA-Table: Create a new entry
		fmt.Println("Not in PA-Table: Creating a new entry")
		PA_Table.AddEntry(vpn, readWrite, true)

		pacEntry := &PACEntry{VPT: vpt, ReadWrite: readWrite, FaultCount: 1, Modified: true}
		cache.Sets[setIndex] = append(cache.Sets[setIndex], pacEntry)
		cache.Visit(setIndex, pacEntry)
	} else {
		// Entry exists in PA-Table, bring it into PA-Cache
		fmt.Println("Entry exists in PA-Table, bringing it into PA-Cache")
		paEntry.FaultCount++

		if !paEntry.ReadWrite && readWrite {
			paEntry.ReadWrite = true
		}
		paEntry.Modified = true

		pacEntry := &PACEntry{VPT: vpt, ReadWrite: paEntry.ReadWrite, FaultCount: paEntry.FaultCount, Modified: paEntry.Modified}
		cache.Sets[setIndex] = append(cache.Sets[setIndex], pacEntry)
		cache.Visit(setIndex, pacEntry)

	}

	// if len(cache.Sets[setIndex]) > cache.WaySize {
	// 	evicted := cache.Evict(setIndex)
	// 	if evicted != nil && evicted.Modified {
	// 		// Write back evicted entry to PA-Table
	// 		evictedVPN := (evicted.VPT << 4) | setIndex
	// 		PA_Table.UpdateEntry(evictedVPN, evicted.ReadWrite, false)

	// 		//PA_Table.UpdateEntry(evicted.VPT<<16, evicted.ReadWrite, false)
	// 	}
	// }

	if len(cache.Sets[setIndex]) > cache.WaySize {
		evicted := cache.Evict(setIndex)
		if evicted != nil {
			evictedVPN := (evicted.VPT << 4) | setIndex

			// Check FaultCount before deciding what to do
			if evicted.FaultCount >= cache.FaultLimit {
				// Remove entry from both PACache and PATable
				cache.RemoveEntry(setIndex, evicted.VPT) // Function to remove from PACache
				PA_Table.DeleteEntry(evictedVPN)         // Function to remove from PATable
			} else {
				// If the entry is modified, write it back to PA-Table
				if evicted.Modified {
					PA_Table.UpdateEntry(evictedVPN, evicted.ReadWrite, false)
				}
			}
		}
	}

}

func (cache *PACache) Evict(setIndex uint64) *PACEntry {
	if len(cache.VisitList[setIndex]) == 0 {
		return nil
	}
	lruEntry := cache.VisitList[setIndex][0]
	cache.VisitList[setIndex] = cache.VisitList[setIndex][1:]

	// Remove from the set as well
	for i, e := range cache.Sets[setIndex] {
		if e.VPT == lruEntry.VPT {
			cache.Sets[setIndex] = append(cache.Sets[setIndex][:i], cache.Sets[setIndex][i+1:]...)
			break
		}
	}
	return lruEntry
}

/*func (s *PATable) Evict() (wayID int, ok bool) { //It removes the least recently used block from the set and returns the wayID of the evicted block. If there is nothing to evict, it returns false
	if s.hasNothingToEvict() {
		return 0, false
	}

	//Original - wayID = s.visitTree.DeleteMin().(*block).wayID
	leastVisited := s.visitList[0]
	wayID = leastVisited.wayID
	s.visitList = s.visitList[1:] //This operation updates s.visitList by setting it to the new slice created by visitList[1:], which excludes the first element (visitList[0]).
	return wayID, true

	//

}*/

/*func (s *setImpl) Visit(wayID int) { // This function will update visitlist for LRU eviction
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

	s.visitList = append(s.visitList, nil)           //Expands the visitList by one to make room for the new block.
	copy(s.visitList[index+1:], s.visitList[index:]) //Shifts elements in the list to the right, starting from the index determined earlier.
	s.visitList[index] = block                       // put block in index position
}
*/

func (cache *PACache) Visit(setIndex uint64, entry *PACEntry) {
	cache.VisitCount++
	entry.LastVisit = cache.VisitCount

	// Remove the entry if already in VisitList
	for i, e := range cache.VisitList[setIndex] {
		if e.VPT == entry.VPT {
			cache.VisitList[setIndex] = append(cache.VisitList[setIndex][:i], cache.VisitList[setIndex][i+1:]...)
			break
		}
	}

	// Reinsert at the correct position (sorted by LastVisit)
	index := sort.Search(len(cache.VisitList[setIndex]), func(i int) bool {
		return cache.VisitList[setIndex][i].LastVisit > entry.LastVisit
	})
	cache.VisitList[setIndex] = append(cache.VisitList[setIndex], nil)
	copy(cache.VisitList[setIndex][index+1:], cache.VisitList[setIndex][index:])
	cache.VisitList[setIndex][index] = entry
}

// Function to print PA Table entries
func (cache *PACache) PrintPATable(PA_Table *PATable) {
	fmt.Println("PA Table Entries:")

	for _, entry := range PA_Table.Entries {
		location := entry.VPN % 100

		if entry.VPN != 0x0 || entry.FaultCount > 0 || entry.ReadWrite {
			fmt.Printf("Loc [%d] --->> VPN: 0x%X, Read/Write: %v, Faults: %d\n", location, entry.VPN, entry.ReadWrite, entry.FaultCount)
		}
	}
}

// Function to print PA-cache entries
func (cache *PACache) PrintPACache() {
	fmt.Println("PA Cache Entries:")
	for setIndex, entries := range cache.Sets {
		for _, entry := range entries {
			fmt.Printf("Set: %d, VPT: 0x%X, Read/Write: %v, Faults: %d\n",
				setIndex, entry.VPT, entry.ReadWrite, entry.FaultCount)
		}
	}
}

// func main() {

// 	paTable := NewPATable(16)

// 	// Initialize PA-Cache with a cache size of 64 and way size of 4 (example)
// 	paCache := NewPACache(64, 4)

// 	fmt.Println("Example 1:")
// 	paCache.HandlePageFault(0x123456789ABC, true, paTable) // Read
// 	paCache.PrintPATable(paTable)
// 	paCache.PrintPACache()

// 	fmt.Println("Example 2:")
// 	paCache.HandlePageFault(0x987654321FFF, false, paTable) // Read
// 	paCache.PrintPATable(paTable)
// 	paCache.PrintPACache()

// 	fmt.Println("Example 3:")
// 	//paCache.HandlePageFault(0x123456789ABC, true,paTable)  // Write
// 	paCache.HandlePageFault(0x111111119ABC, true, paTable) // Write
// 	paCache.PrintPATable(paTable)
// 	paCache.PrintPACache()

// 	fmt.Println("Example 4:")
// 	//paCache.HandlePageFault(0x123456789ABC, true,paTable)  // Write
// 	paCache.HandlePageFault(0x122222229ABC, true, paTable) // Write
// 	paCache.PrintPATable(paTable)
// 	paCache.PrintPACache()

// 	fmt.Println("Example 5:")
// 	//paCache.HandlePageFault(0x123456789ABC, true,paTable)  // Write
// 	paCache.HandlePageFault(0x133333339ABC, true, paTable) // Write
// 	paCache.PrintPATable(paTable)
// 	paCache.PrintPACache()

// 	fmt.Println("Example 6:")
// 	paCache.HandlePageFault(0x1444444449ABC, true, paTable) // Write
// 	paCache.PrintPATable(paTable)
// 	paCache.PrintPACache()

// 	fmt.Println("Example 7:")
// 	paCache.HandlePageFault(0x111111119ABC, true, paTable) // Write
// 	paCache.PrintPATable(paTable)
// 	paCache.PrintPACache()

// 	fmt.Println("Example 8:")
// 	paCache.HandlePageFault(0x1555555559ABC, true, paTable) // Write
// 	paCache.PrintPATable(paTable)
// 	paCache.PrintPACache()

// 	fmt.Println("Example 9:")
// 	paCache.HandlePageFault(0x111111119ABC, true, paTable) // Write
// 	paCache.PrintPATable(paTable)
// 	paCache.PrintPACache()

// 	// fmt.Println("Example 9:")
// 	// //paCache.HandlePageFault(0x123456789ABC, true,paTable)  // Write
// 	// paCache.HandlePageFault(0x133333339ABC, true, paTable) // Write
// 	// paCache.PrintPATable(paTable)
// 	// paCache.PrintPACache()

// 	// fmt.Println("Example 10:")
// 	// paCache.HandlePageFault(0x1666666669ABC, true, paTable) // Write
// 	// paCache.PrintPATable(paTable)
// 	// paCache.PrintPACache()

// }
