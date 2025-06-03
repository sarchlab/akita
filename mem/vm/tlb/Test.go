package tlb

// import (
// 	"fmt"

// 	"github.com/sarchlab/akita/v3/mem/vm"
// 	"github.com/sarchlab/akita/v3/mem/vm/tlb/internal"
// )

// // PAEntry represents an entry in the PA-Table
// type PAEntry struct {
// 	page       vm.Page
// 	VPN        uint64
// 	ReadWrite  bool // false for read, true for write
// 	FaultCount int
// 	//Modified   bool // Tracks if an entry has been modified before eviction
// }

// // PA-Table (Direct-Mapped Cache)
// type PATable struct {
// 	Entries   []PAEntry
// 	TableSize int
// }

// func NewPATable(tableSize int) *PATable {
// 	return &PATable{
// 		Entries:   make([]PAEntry, tableSize),
// 		TableSize: tableSize,
// 	}
// }

// func (pa *PATable) GetEntry(vpn uint64) (*PAEntry, bool) {
// 	index := vpn % uint64(pa.TableSize)
// 	entry := &pa.Entries[index]
// 	if entry.VPN == vpn {
// 		return entry, true
// 	}

// 	return nil, false
// }

// func (pa *PATable) UpdateEntry(vpn uint64, readWrite bool, faultIncrement bool) {
// 	index := vpn % uint64(pa.TableSize)
// 	entry := &pa.Entries[index]

// 	if entry.VPN == vpn {
// 		if faultIncrement {
// 			entry.FaultCount++
// 		}
// 		if !entry.ReadWrite && readWrite {
// 			entry.ReadWrite = true // Once set to write, it remains write
// 		}
// 		//entry.Modified = true
// 	} else {
// 		// Replace existing entry in the direct-mapped slot
// 		*entry = PAEntry{VPN: vpn, ReadWrite: readWrite, FaultCount: 1}
// 	}
// }

// func (pa *PATable) deleteEntry(vpn uint64) {
// 	index := vpn % uint64(pa.TableSize)
// 	entry := &pa.Entries[index]
// 	if entry.VPN == vpn {
// 		pa.Entries[index] = PAEntry{}
// 	}
// }

// type PACache struct {
// 	CacheSize  uint64
// 	WaySize    uint64 //numWays
// 	NumSets    int
// 	FaultLimit uint8
// 	Sets       []internal.Set
// }

// func NewPACache(cacheSize uint64, waySize uint64) *PACache {
// 	cache := &PACache{
// 		CacheSize: cacheSize,
// 		WaySize:   waySize,
// 		NumSets:   int(cacheSize / waySize),
// 		Sets:      make([]internal.Set, int(cacheSize/waySize)),
// 	}

// 	for i := 0; i < cache.NumSets; i++ {
// 		set := internal.NewSet(int(waySize))
// 		cache.Sets[i] = set
// 	}

// 	return cache
// }

// func (cache *PACache) GetSetIndex(vAddr uint64) uint64 {
// 	return (vAddr >> 12) & 0xF // Extract 4-bit set index
// }

// func (cache *PACache) GetVPN(vAddr uint64) uint64 {
// 	return vAddr >> 12 // Extract VPT after removing index bits
// }

// func (cache *PACache) GetVPT(vAddr uint64) uint64 {
// 	return vAddr >> 16 // Extract VPT after removing index bits
// }

// // func (cache *PACache) vAddrToSetID(vAddr uint64) (setID int) {
// // 	return int(vAddr / uint64(12) % uint64(cache.NumSets))
// // 	//return (vAddr >> 12) & 0xF
// // }

// func (cache *PACache) HandlePageFault(page vm.Page, pid vm.PID, readWrite bool, PA_Table *PATable) {

// 	vpn := cache.GetVPN(page.VAddr)

// 	//setID := cache.vAddrToSetID(page.VAddr)
// 	setID := cache.GetSetIndex(page.VAddr)

// 	set := cache.Sets[setID]

// 	fmt.Printf("Set Id =%d \n", setID)
// 	block, found := set.PACLookup(pid, page.VAddr)

// 	if found {

// 		// block.FaultCount++

// 		// if !block.ReadWrite && readWrite {
// 		// 	block.ReadWrite = true
// 		// }
// 		fmt.Println("%d \n", block)
// 		fmt.Println("Cache Hit: Updated entry in PA-Cache")

// 		// if block.FaultCount >= cache.FaultLimit {
// 		// 	fmt.Println("Fault threshold reached, deleting entry from PA-Cache and putting into PA-Table")
// 		// 	PA_Table.deleteEntry(vpn)

// 		// 	wayID, ok := cache.Sets[setID].Evict()
// 		// 	if !ok {
// 		// 		panic("failed to evict")
// 		// 	}
// 		// 	set.Update(wayID, page)
// 		// 	set.Visit(wayID)
// 		// 	fmt.Println("new scheme")
// 		// }
// 		// return

// 	} else { //PA-Cache miss
// 		fmt.Println("Cache Miss: Fetching from PA-Table")
// 		paEntry, exists := PA_Table.GetEntry(vpn)
// 		if !exists {
// 			// Not in PA-Table: Create a new entry
// 			fmt.Println("Not in PA-Table: Creating a new entry")
// 			PA_Table.UpdateEntry(vpn, readWrite, true)
// 			//set.AddEntry(page, readWrite, 1, PA_Table)

// 			set.AddEntry(page, readWrite, 1)
// 		} else {
// 			fmt.Println("Entry exists in PA-Table, bringing it into PA-Cache")
// 			set.AddEntry(paEntry.page, paEntry.ReadWrite, uint64(paEntry.FaultCount))
// 		} //end of else exists

// 	} //end of ELSE

// }

// /* Function to print PA Table entries */
// func (cache *PACache) PrintPATable(PA_Table *PATable) {
// 	fmt.Println("PA Table Entries:")
// 	for _, entry := range PA_Table.Entries {
// 		fmt.Printf("VPN: 0x%X, Read/Write: %v, Faults: %d\n", entry.VPN, entry.ReadWrite, entry.FaultCount)
// 	}
// }

// ///* Function to print PA-cache entries */

// func (cache *PACache) PrintPACache() {
// 	fmt.Println("PA Cache Entries:")
// 	for setIndex, set := range cache.Sets {
// 		// Type assertion to access internal fields
// 		setImpl, ok := set.(*internal.SetImpl)
// 		if !ok {
// 			fmt.Printf("Error: set at index %d is not of type *SetImpl\n", setIndex)
// 			continue
// 		}

// 		// Use the exported method to access blocks
// 		for _, blk := range setImpl.GetBlocks() {
// 			fmt.Printf("Set: %d, VAddr: 0x%X, Read/Write: %v, Faults: %d\n", setIndex, blk.Page.VAddr, blk.ReadWrite, blk.FaultCount)
// 		}

// 	}
// }
