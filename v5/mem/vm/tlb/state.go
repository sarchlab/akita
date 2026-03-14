package tlb

import (
	"github.com/sarchlab/akita/v5/mem/vm"
	"github.com/sarchlab/akita/v5/mem/vm/lruset"
	"github.com/sarchlab/akita/v5/sim"
)

// blockState is a serializable representation of an internal block.
type blockState struct {
	Page  vm.Page `json:"page"`
	WayID int     `json:"way_id"`
}

// setState is the serializable representation of one TLB set.
type setState struct {
	Blocks []blockState `json:"blocks"`
	LRU    lruset.Set   `json:"lru"`
}

// mshrEntryState is a serializable representation of an mshrEntry.
type mshrEntryState struct {
	PID            uint32              `json:"pid"`
	VAddr          uint64              `json:"vaddr"`
	Requests       []vm.TranslationReq `json:"requests"`
	HasReqToBottom bool                `json:"has_req_to_bottom"`
	ReqToBottom    vm.TranslationReq   `json:"req_to_bottom"`
	Page           vm.Page             `json:"page"`
}

// pipelineTLBReqState is a serializable pipeline item.
type pipelineTLBReqState struct {
	Msg vm.TranslationReq `json:"msg"`
}

// --- Free functions for Set operations (delegating to lruset) ---

func setLookup(s *setState, pid vm.PID, vAddr uint64) (wayID int, page vm.Page, found bool) {
	key := lruset.KeyString(uint64(pid), vAddr)
	wayID, ok := lruset.Lookup(&s.LRU, key)
	if !ok {
		return 0, vm.Page{}, false
	}
	block := s.Blocks[wayID]
	return block.WayID, block.Page, true
}

func setUpdate(s *setState, wayID int, page vm.Page) {
	block := &s.Blocks[wayID]
	oldKey := lruset.KeyString(uint64(block.Page.PID), block.Page.VAddr)
	block.Page = page
	newKey := lruset.KeyString(uint64(page.PID), page.VAddr)
	lruset.UpdateKey(&s.LRU, wayID, oldKey, newKey)
}

func setEvict(s *setState) (wayID int, ok bool) {
	return lruset.Evict(&s.LRU)
}

func setVisit(s *setState, wayID int) {
	lruset.Visit(&s.LRU, wayID)
}

func initSets(numSets, numWays int) []setState {
	sets := make([]setState, numSets)
	for i := 0; i < numSets; i++ {
		s := setState{
			Blocks: make([]blockState, numWays),
			LRU:    lruset.NewSet(numWays),
		}
		for j := 0; j < numWays; j++ {
			s.Blocks[j] = blockState{WayID: j}
		}
		sets[i] = s
	}
	return sets
}

// --- Free functions for MSHR operations ---

func mshrGetEntry(entries []mshrEntryState, pid vm.PID, vAddr uint64) (int, bool) {
	for i, e := range entries {
		if vm.PID(e.PID) == pid && e.VAddr == vAddr {
			return i, true
		}
	}
	return -1, false
}

func mshrAdd(entries []mshrEntryState, capacity int, pid vm.PID, vAddr uint64) ([]mshrEntryState, int) {
	for _, e := range entries {
		if vm.PID(e.PID) == pid && e.VAddr == vAddr {
			panic("entry already in mshr")
		}
	}
	if len(entries) >= capacity {
		panic("MSHR is full")
	}
	entry := mshrEntryState{
		PID:   uint32(pid),
		VAddr: vAddr,
	}
	entries = append(entries, entry)
	return entries, len(entries) - 1
}

func mshrRemove(entries []mshrEntryState, pid vm.PID, vAddr uint64) []mshrEntryState {
	for i, e := range entries {
		if vm.PID(e.PID) == pid && e.VAddr == vAddr {
			return append(entries[:i], entries[i+1:]...)
		}
	}
	panic("trying to remove a non-exist entry")
}

func mshrIsFull(entries []mshrEntryState, capacity int) bool {
	return len(entries) >= capacity
}

func mshrIsEmpty(entries []mshrEntryState) bool {
	return len(entries) == 0
}

func mshrIsEntryPresent(entries []mshrEntryState, pid vm.PID, vAddr uint64) bool {
	_, found := mshrGetEntry(entries, pid, vAddr)
	return found
}

// --- Free function for address mapping ---

func findTranslationPort(spec Spec, vAddr uint64) sim.RemotePort {
	switch spec.AddrMapperKind {
	case "single":
		if len(spec.AddrMapperPorts) != 1 {
			panic("single address mapper requires exactly 1 port")
		}
		return spec.AddrMapperPorts[0]
	case "interleaved":
		if len(spec.AddrMapperPorts) == 0 {
			panic("interleaved address mapper requires at least 1 port")
		}
		number := vAddr / spec.AddrMapperInterleavingSize % uint64(len(spec.AddrMapperPorts))
		return spec.AddrMapperPorts[number]
	default:
		panic("invalid address mapper kind: " + spec.AddrMapperKind)
	}
}
