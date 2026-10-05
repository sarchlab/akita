package tlb

import (
	"github.com/sarchlab/akita/v5/mem"
	"github.com/sarchlab/akita/v5/mem/mshr"
	"github.com/sarchlab/akita/v5/mem/vm"
	"github.com/sarchlab/akita/v5/mem/vm/lruset"
	"github.com/sarchlab/akita/v5/sim/messaging"
	"github.com/sarchlab/akita/v5/sim/modeling/ticking"
	"github.com/sarchlab/akita/v5/sim/queueing"
	"github.com/sarchlab/akita/v5/sim/timing"
)

// Spec contains immutable configuration for the TLB.
type Spec struct {
	Freq           timing.Freq `json:"freq"`
	NumSets        int         `json:"num_sets"`
	NumWays        int         `json:"num_ways"`
	Log2PageSize   uint64      `json:"log2_page_size"`
	NumReqPerCycle int         `json:"num_req_per_cycle"`
	MSHRSize       int         `json:"mshr_size"`
	Latency        int         `json:"latency"`
}

// pageSize returns the size of a page, 2^Log2PageSize bytes.
func (s Spec) pageSize() uint64 {
	return 1 << s.Log2PageSize
}

// Ports holds the TLB's ports.
type Ports struct {
	// Top receives translation requests and returns their responses.
	Top messaging.Port `akita:"role=github.com/sarchlab/akita/v5/mem/vm/vmprotocol.responder"`

	// Bottom sends translation requests for misses to the next level.
	Bottom messaging.Port `akita:"role=github.com/sarchlab/akita/v5/mem/vm/vmprotocol.requester"`

	// Control receives enable, pause, drain, flush, and reset commands.
	Control messaging.Port `akita:"role=github.com/sarchlab/akita/v5/mem/memcontrolprotocol.responder"`
}

// middlewares holds the TLB's behavior, run in field order every cycle.
type middlewares struct {
	// Ctrl handles control commands.
	Ctrl *ctrlMiddleware

	// TLB looks up translations, tracks misses, and responds.
	TLB *tlbMiddleware
}

// Resources holds the external objects wired into the TLB. The translation
// provider mapper resolves the remote port that serves the translation for a
// given virtual address.
type Resources struct {
	TranslationProviderMapper mem.AddressToPortMapper `json:"-"`
}

const (
	tlbStateEnable = "enable"
	tlbStatePause  = "pause"
	tlbStateDrain  = "drain"
)

// state contains mutable runtime data for the TLB.
type state struct {
	TLBState           string                                 `json:"tlb_state"`
	PendingDrainRsp    bool                                   `json:"pending_drain_rsp"`
	CurrentCmdID       uint64                                 `json:"current_cmd_id"`
	CurrentCmdSrc      messaging.RemotePort                   `json:"current_cmd_src"`
	Sets               []setState                             `json:"sets"`
	MSHREntries        []mshrEntryState                       `json:"mshr_entries"`
	HasRespondingMSHR  bool                                   `json:"has_responding_mshr"`
	RespondingMSHRData mshrEntryState                         `json:"responding_mshr_data"`
	Pipeline           queueing.Pipeline[pipelineTLBReqState] `json:"pipeline"`
	BufferItems        queueing.Buffer[pipelineTLBReqState]   `json:"buffer_items"`
}

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
	PID            uint32          `json:"pid"`
	VAddr          uint64          `json:"vaddr"`
	Requests       []messaging.Msg `json:"requests"`
	HasReqToBottom bool            `json:"has_req_to_bottom"`
	ReqToBottom    messaging.Msg   `json:"req_to_bottom"`
	Page           vm.Page         `json:"page"`
}

// GetPID returns the PID of the MSHR entry.
func (e mshrEntryState) GetPID() uint32 { return e.PID }

// GetAddress returns the virtual address of the MSHR entry.
func (e mshrEntryState) GetAddress() uint64 { return e.VAddr }

// pipelineTLBReqState is a serializable pipeline item. PipelineTaskID is the ID
// of the tracing pipeline subtask opened (as a child of req_in) when the request
// enters the pipeline at retrieve and closed when it is popped at lookup.
type pipelineTLBReqState struct {
	Msg            messaging.Msg `json:"msg"`
	PipelineTaskID uint64        `json:"pipeline_task_id"`
}

// --- Free functions for Set operations (delegating to lruset) ---

func setLookup(s *setState, pid vm.PID, vAddr uint64) (wayID int, page vm.Page, found bool) {
	key := lruset.KeyString(uint64(pid), vAddr)
	wayID, ok := s.LRU.Lookup(key)
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
	s.LRU.UpdateKey(wayID, oldKey, newKey)
}

func setEvict(s *setState) (wayID int, ok bool) {
	return s.LRU.Evict()
}

func setVisit(s *setState, wayID int) {
	s.LRU.Visit(wayID)
}

func initSets(numSets, numWays int) []setState {
	sets := make([]setState, numSets)
	for i := 0; i < numSets; i++ {
		s := setState{
			Blocks: make([]blockState, numWays),
			LRU:    lruset.MakeSet(numWays),
		}
		for j := 0; j < numWays; j++ {
			s.Blocks[j] = blockState{WayID: j}
		}
		sets[i] = s
	}
	return sets
}

// --- Free functions for MSHR operations (delegating to shared mshr package) ---

func mshrGetEntry(entries []mshrEntryState, pid vm.PID, vAddr uint64) (int, bool) {
	return mshr.Find(entries, pid, vAddr)
}

func mshrAdd(entries []mshrEntryState, capacity int, pid vm.PID, vAddr uint64) ([]mshrEntryState, int) {
	if mshr.IsPresent(entries, pid, vAddr) {
		panic("entry already in mshr")
	}

	if mshr.IsFull(entries, capacity) {
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
	return mshr.Remove(entries, pid, vAddr)
}

func mshrIsFull(entries []mshrEntryState, capacity int) bool {
	return mshr.IsFull(entries, capacity)
}

func mshrIsEmpty(entries []mshrEntryState) bool {
	return mshr.IsEmpty(entries)
}

func mshrIsEntryPresent(entries []mshrEntryState, pid vm.PID, vAddr uint64) bool {
	return mshr.IsPresent(entries, pid, vAddr)
}

// --- Free function for address mapping ---

func findTranslationPort(
	mapper mem.AddressToPortMapper,
	vAddr uint64,
) messaging.RemotePort {
	if mapper == nil {
		panic("no translation provider mapper configured")
	}

	return mapper.Find(vAddr)
}

// Comp is the TLB component.
type Comp = ticking.Component[Spec, state, Resources, Ports, middlewares]
