package vm

// import (
// 	"fmt"
// )

// // PAEntry represents an entry in the PA-Table
// type PAEntry struct {
// 	VPN        uint64
// 	ReadWrite  bool // false for read, true for write
// 	FaultCount uint8
// 	Modified   bool // Tracks if an entry has been modified before eviction
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
// 		entry.Modified = true
// 	} else {
// 		// Replace existing entry in the direct-mapped slot
// 		*entry = PAEntry{VPN: vpn, ReadWrite: readWrite, FaultCount: 1, Modified: true}
// 	}
// }

// type PACEntry struct {
// 	VPT        uint64
// 	ReadWrite  bool // false for read, true for write
// 	FaultCount uint8
// 	Modified   bool // Tracks if an entry has been modified before eviction
// }

// // PA-Cache (Write-Allocate + Write-Back Cache)
// type PACache struct {
// 	Sets       map[uint64][]*PACEntry
// 	CacheSize  int
// 	WaySize    int
// 	FaultLimit uint8
// }

// func NewPACache(cacheSize, waySize int) *PACache {
// 	return &PACache{
// 		Sets:       make(map[uint64][]*PACEntry),
// 		CacheSize:  cacheSize,
// 		WaySize:    waySize,
// 		FaultLimit: 2, // Example threshold
// 	}
// }

// // GetSetIndex computes the set index from VPN
// func (cache *PACache) GetSetIndex(vpn uint64) uint64 {
// 	return (vpn >> 12) & 0xF // Extract 4-bit set index
// }

// // GetVPT extracts the Virtual Page Tag (VPT)
// func (cache *PACache) GetVPT(vpn uint64) uint64 {
// 	return vpn >> 16 // Extract VPT after removing index bits
// }

// // GetVPT extracts the Virtual Page Tag (VPT)
// func (cache *PACache) GetVPN(vpn uint64) uint64 {
// 	return vpn >> 12 // Extract VPT after removing index bits
// }
// func (cache *PACache) HandlePageFault(vAddr uint64, readWrite bool, PA_Table *PATable) {
// 	setIndex := cache.GetSetIndex(vAddr)
// 	vpt := cache.GetVPT(vAddr)
// 	vpn := cache.GetVPN(vAddr)
// 	// Check if entry exists in PA-Cache
// 	for _, entry := range cache.Sets[setIndex] {
// 		if entry.VPT == vpt {
// 			// Cache Hit: Update fault counter and read/write bit
// 			entry.FaultCount++
// 			if !entry.ReadWrite && readWrite {
// 				entry.ReadWrite = true
// 			}
// 			fmt.Println("Cache Hit: Updated entry in PA-Cache")
// 			// If fault counter reaches threshold remove from both PA-Cache and put into PA Table
// 			if entry.FaultCount >= cache.FaultLimit {
// 				fmt.Println("Fault threshold reached, deleting entry from PA-Cache and putting into PA-Table")
// 				PA_Table.UpdateEntry(vpn, entry.ReadWrite, false)
// 				cache.Sets[setIndex] = cache.Sets[setIndex][:len(cache.Sets[setIndex])-1] // Remove from cache
// 				fmt.Println("new scheme")
// 			}
// 			return
// 		}
// 	}

// 	// Cache Miss: Fetch from PA-Table (Write-Allocate)
// 	fmt.Println("Cache Miss: Fetching from PA-Table")

// 	paEntry, exists := PA_Table.GetEntry(vpn)
// 	if !exists {
// 		// Not in PA-Table: Create a new entry
// 		fmt.Println("Not in PA-Table: Creating a new entry")
// 		PA_Table.UpdateEntry(vpn, readWrite, true)

// 		// Convert PAEntry to PACEntry
// 		pacEntry := &PACEntry{VPT: vpt, ReadWrite: readWrite, FaultCount: 1, Modified: true}

// 		cache.Sets[setIndex] = append(cache.Sets[setIndex], pacEntry)
// 	} else {
// 		// Entry exists in PA-Table, bring it into PA-Cache
// 		fmt.Println("Entry exists in PA-Table, bringing it into PA-Cache")
// 		paEntry.FaultCount++
// 		if readWrite {
// 			paEntry.ReadWrite = true
// 		}
// 		paEntry.Modified = true

// 		// Convert PAEntry to PACEntry
// 		pacEntry := &PACEntry{VPT: vpt, ReadWrite: paEntry.ReadWrite, FaultCount: paEntry.FaultCount, Modified: paEntry.Modified}
// 		cache.Sets[setIndex] = append(cache.Sets[setIndex], pacEntry)
// 	}

// 	// Insert into PA-Cache (Apply LRU if full)
// 	if len(cache.Sets[setIndex]) >= cache.WaySize {
// 		// Evict LRU entry if full
// 		evicted := cache.Sets[setIndex][0]
// 		cache.Sets[setIndex] = cache.Sets[setIndex][1:] // Remove LRU

// 		// Write back if modified
// 		if evicted.Modified {
// 			fmt.Println("Evicting entry, writing back to PA-Table")
// 			PA_Table.UpdateEntry(evicted.VPT<<16, evicted.ReadWrite, false) // Convert VPT back to VPN
// 		}
// 	}
// }

// // Function to print PA Table entries
// func (cache *PACache) PrintPATable(PA_Table *PATable) {
// 	fmt.Println("PA Table Entries:")
// 	for _, entry := range PA_Table.Entries {
// 		fmt.Printf("VPN: 0x%X, Read/Write: %v, Faults: %d\n", entry.VPN, entry.ReadWrite, entry.FaultCount)
// 	}
// }

// // Function to print PA-cache entries
// func (cache *PACache) PrintPACache() {
// 	fmt.Println("PA Cache Entries:")
// 	for setIndex, entries := range cache.Sets {
// 		for _, entry := range entries {
// 			fmt.Printf("Set: %d, VPT: 0x%X, Read/Write: %v, Faults: %d\n",
// 				setIndex, entry.VPT, entry.ReadWrite, entry.FaultCount)
// 		}
// 	}
// }

// // func main() {

// // 	paTable := NewPATable(16)

// // 	// Initialize PA-Cache with a cache size of 64 and way size of 4 (example)
// // 	paCache := NewPACache(64, 4)

// // 	fmt.Println("Example 1:")
// // 	//paCache.HandlePageFault(0x123456789ABC, IsWrite(),paTable) // Read
// // 	paCache.PrintPATable(paTable)
// // 	paCache.PrintPACache()

// // 	fmt.Println("Example 2:")
// // 	paCache.HandlePageFault(0x987654321FFF, false, paTable) // Read
// // 	paCache.PrintPATable(paTable)
// // 	paCache.PrintPACache()

// // 	fmt.Println("Example 3:")
// // 	//paCache.HandlePageFault(0x123456789ABC, true,paTable)  // Write
// // 	paCache.HandlePageFault(0x111111119ABC, true, paTable) // Write
// // 	paCache.PrintPATable(paTable)
// // 	paCache.PrintPACache()

// // 	fmt.Println("Example 4:")
// // 	//paCache.HandlePageFault(0x123456789ABC, true,paTable)  // Write
// // 	paCache.HandlePageFault(0x122222229ABC, true, paTable) // Write
// // 	paCache.PrintPATable(paTable)
// // 	paCache.PrintPACache()

// // 	fmt.Println("Example 5:")
// // 	//paCache.HandlePageFault(0x123456789ABC, true,paTable)  // Write
// // 	paCache.HandlePageFault(0x133333339ABC, true, paTable) // Write
// // 	paCache.PrintPATable(paTable)
// // 	paCache.PrintPACache()

// // 	fmt.Println("Example 6:")
// // 	//paCache.HandlePageFault(0x123456789ABC, true,paTable)  // Write
// // 	paCache.HandlePageFault(0x1444444449ABC, true, paTable) // Write
// // 	paCache.PrintPATable(paTable)
// // 	paCache.PrintPACache()

// // }
