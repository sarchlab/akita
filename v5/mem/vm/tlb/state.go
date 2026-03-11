package tlb

import (
	"fmt"
	"sort"

	"github.com/sarchlab/akita/v5/mem/vm"
	"github.com/sarchlab/akita/v5/sim"
)

// blockState is a serializable representation of an internal block.
type blockState struct {
	Page      vm.Page `json:"page"`
	WayID     int     `json:"way_id"`
	LastVisit uint64  `json:"last_visit"`
}

// setState is the serializable representation of one TLB set.
type setState struct {
	Blocks     []blockState   `json:"blocks"`
	VisitList  []int          `json:"visit_list"`
	VisitCount uint64         `json:"visit_count"`
	VAddrMap   map[string]int `json:"vaddr_map"` // "pid+vaddr" -> wayID
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

// pipelineStageState captures one non-nil pipeline slot.
type pipelineStageState struct {
	Lane      int                 `json:"lane"`
	Stage     int                 `json:"stage"`
	Item      pipelineTLBReqState `json:"item"`
	CycleLeft int                 `json:"cycle_left"`
}

// --- Free functions for Set operations ---

func setKeyString(pid vm.PID, vAddr uint64) string {
	return fmt.Sprintf("%d%016x", pid, vAddr)
}

func setLookup(s *setState, pid vm.PID, vAddr uint64) (wayID int, page vm.Page, found bool) {
	key := setKeyString(pid, vAddr)
	wayID, ok := s.VAddrMap[key]
	if !ok {
		return 0, vm.Page{}, false
	}
	block := s.Blocks[wayID]
	return block.WayID, block.Page, true
}

func setUpdate(s *setState, wayID int, page vm.Page) {
	block := &s.Blocks[wayID]
	// Remove old mapping
	oldKey := setKeyString(block.Page.PID, block.Page.VAddr)
	delete(s.VAddrMap, oldKey)
	// Update block
	block.Page = page
	// Add new mapping
	newKey := setKeyString(page.PID, page.VAddr)
	s.VAddrMap[newKey] = wayID
}

func setEvict(s *setState) (wayID int, ok bool) {
	if len(s.VisitList) == 0 {
		return 0, false
	}
	wayID = s.VisitList[0]
	s.VisitList = s.VisitList[1:]
	return wayID, true
}

func setVisit(s *setState, wayID int) {
	// Remove wayID from visit list if present
	for i, w := range s.VisitList {
		if w == wayID {
			s.VisitList = append(s.VisitList[:i], s.VisitList[i+1:]...)
			break
		}
	}
	s.VisitCount++
	s.Blocks[wayID].LastVisit = s.VisitCount

	// Insert in sorted position by lastVisit
	targetVisit := s.VisitCount
	index := sort.Search(len(s.VisitList), func(i int) bool {
		return s.Blocks[s.VisitList[i]].LastVisit > targetVisit
	})
	s.VisitList = append(s.VisitList, 0)
	copy(s.VisitList[index+1:], s.VisitList[index:])
	s.VisitList[index] = wayID
}

func initSets(numSets, numWays int) []setState {
	sets := make([]setState, numSets)
	for i := 0; i < numSets; i++ {
		s := setState{
			Blocks:   make([]blockState, numWays),
			VAddrMap: make(map[string]int),
		}
		for j := 0; j < numWays; j++ {
			s.Blocks[j] = blockState{WayID: j}
			// Initialize visitList with all ways visited in order
		}
		// Visit each way to populate visitList (LRU initialization)
		s.VisitList = make([]int, 0, numWays)
		for j := 0; j < numWays; j++ {
			setVisit(&s, j)
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

// --- Free functions for Pipeline operations ---

func pipelineCanAccept(stages []pipelineStageState, width int, numStages int) bool {
	// Check if the last stage has room
	lastStage := numStages - 1
	count := 0
	for _, s := range stages {
		if s.Stage == lastStage {
			count++
		}
	}
	return count < width
}

func pipelineAccept(stages []pipelineStageState, width int, numStages int, item pipelineTLBReqState) []pipelineStageState {
	// Find an empty lane at the last stage
	lastStage := numStages - 1
	usedLanes := make(map[int]bool)
	for _, s := range stages {
		if s.Stage == lastStage {
			usedLanes[s.Lane] = true
		}
	}
	lane := 0
	for usedLanes[lane] {
		lane++
	}
	stages = append(stages, pipelineStageState{
		Lane:      lane,
		Stage:     lastStage,
		Item:      item,
		CycleLeft: 1,
	})
	return stages
}

func pipelineTick(stages []pipelineStageState, buffer *[]pipelineTLBReqState) ([]pipelineStageState, bool) {
	madeProgress := false
	remaining := make([]pipelineStageState, 0, len(stages))

	for i := range stages {
		s := stages[i]
		if s.CycleLeft > 0 {
			s.CycleLeft--
			madeProgress = true
		}
		if s.CycleLeft == 0 {
			if s.Stage == 0 {
				// Item exits pipeline into buffer
				*buffer = append(*buffer, s.Item)
				madeProgress = true
				continue // don't keep in stages
			}
			// Move to next stage
			s.Stage--
			s.CycleLeft = 1
			madeProgress = true
		}
		remaining = append(remaining, s)
	}

	stages = remaining
	return stages, madeProgress
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
