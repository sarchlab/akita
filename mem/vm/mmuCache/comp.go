package mmuCache

import (
	"github.com/sarchlab/akita/v5/mem/vm"
	"github.com/sarchlab/akita/v5/mem/vm/lruset"
	"github.com/sarchlab/akita/v5/messaging"
	"github.com/sarchlab/akita/v5/modeling"
	"github.com/sarchlab/akita/v5/timing"
)

// Spec contains immutable configuration for the mmuCache.
type Spec struct {
	Freq            timing.Freq `json:"freq"`
	NumBlocks       int         `json:"num_blocks"`
	NumLevels       int         `json:"num_levels"`
	PageSize        uint64      `json:"page_size"`
	Log2PageSize    uint64      `json:"log2_page_size"`
	NumReqPerCycle  int         `json:"num_req_per_cycle"`
	LatencyPerLevel uint64      `json:"latency_per_level"`

	TopPortBufferSize     int `json:"top_port_buffer_size"`
	BottomPortBufferSize  int `json:"bottom_port_buffer_size"`
	ControlPortBufferSize int `json:"control_port_buffer_size"`
}

// Resources holds the external wiring referenced by the mmuCache: the remote
// ports it forwards translation requests to and responses back from.
type Resources struct {
	LowModulePort messaging.RemotePort `json:"low_module_port"`
	UpModulePort  messaging.RemotePort `json:"up_module_port"`
}

const (
	mmuCacheStateEnable = "enable"
	mmuCacheStatePause  = "pause"
	mmuCacheStateDrain  = "drain"
)

// State contains mutable runtime data for the mmuCache.
type State struct {
	CurrentState    string               `json:"current_state"`
	PendingDrainRsp bool                 `json:"pending_drain_rsp"`
	CurrentCmdID    uint64               `json:"current_cmd_id"`
	CurrentCmdSrc   messaging.RemotePort `json:"current_cmd_src"`
	Table           []setState           `json:"table"`

	// InflightBottomReqs counts translation requests forwarded to the low
	// module for which no response has returned yet. A Drain is not complete
	// while any of these are outstanding, otherwise the late response would
	// land (updating the table and replying upward) after the caller was told
	// the cache had drained.
	InflightBottomReqs int `json:"inflight_bottom_reqs"`
}

// blockState is a serializable snapshot of a single cache block.
type blockState struct {
	PID   uint64 `json:"pid"`
	Seg   uint64 `json:"seg"`
	WayID int    `json:"way_id"`
}

// setState is the serializable snapshot of one mmuCache set (level).
type setState struct {
	Blocks []blockState `json:"blocks"`
	LRU    lruset.Set   `json:"lru"`
}

// --- Free functions for Set operations (delegating to lruset) ---

func setLookup(s *setState, pid vm.PID, seg uint64) (wayID int, found bool) {
	key := lruset.KeyString(uint64(pid), seg)
	wayID, ok := s.LRU.Lookup(key)
	if !ok {
		return 0, false
	}
	return s.Blocks[wayID].WayID, true
}

func setUpdate(s *setState, wayID int, pid vm.PID, seg uint64) {
	block := &s.Blocks[wayID]
	oldKey := lruset.KeyString(block.PID, block.Seg)
	block.PID = uint64(pid)
	block.Seg = seg
	newKey := lruset.KeyString(uint64(pid), seg)
	s.LRU.UpdateKey(wayID, oldKey, newKey)
}

func setEvict(s *setState) (wayID int, ok bool) {
	return s.LRU.Evict()
}

// setRemove drops the cached entry keyed by (pid, seg), turning future
// lookups for that key into misses. It is a no-op if the key is absent.
func setRemove(s *setState, pid vm.PID, seg uint64) {
	s.LRU.Remove(lruset.KeyString(uint64(pid), seg))
}

func setVisit(s *setState, wayID int) {
	s.LRU.Visit(wayID)
}

func initSets(numLevels, numBlocks int) []setState {
	sets := make([]setState, numLevels)
	for i := 0; i < numLevels; i++ {
		s := setState{
			Blocks: make([]blockState, numBlocks),
			LRU:    lruset.NewSet(numBlocks),
		}
		for j := 0; j < numBlocks; j++ {
			s.Blocks[j] = blockState{WayID: j}
		}
		sets[i] = s
	}
	return sets
}

// Comp is the mmuCache component.
type Comp = modeling.Component[Spec, State, Resources]
