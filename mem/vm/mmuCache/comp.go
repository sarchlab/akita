package mmuCache

import (
	"github.com/sarchlab/akita/v5/mem/vm"
	"github.com/sarchlab/akita/v5/mem/vm/lruset"
	"github.com/sarchlab/akita/v5/sim/messaging"
	"github.com/sarchlab/akita/v5/sim/modeling/ticking"
	"github.com/sarchlab/akita/v5/sim/timing"
)

// Spec contains immutable configuration for the mmuCache.
type Spec struct {
	Freq            timing.Freq `json:"freq"`
	NumBlocks       int         `json:"num_blocks"`
	NumLevels       int         `json:"num_levels"`
	PageSize        uint64      `json:"page_size"`
	Log2PageSize    uint64      `json:"log2_page_size"`
	NumReqPerCycle  int         `json:"num_req_per_cycle"`
	LatencyPerLevel int         `json:"latency_per_level"`

	// LowModule is the downstream translation provider address.
	LowModule messaging.RemotePort `json:"low_module"`
	// UpModule is the upstream translation response destination.
	UpModule messaging.RemotePort `json:"up_module"`
}

// Ports holds the mmuCache's ports.
type Ports struct {
	// Top receives translation requests and returns their responses.
	Top messaging.Port `akita:"role=github.com/sarchlab/akita/v5/mem/vm/addrtransprotocol.responder"`

	// Bottom forwards translation requests for misses to the low module.
	Bottom messaging.Port `akita:"role=github.com/sarchlab/akita/v5/mem/vm/addrtransprotocol.requester"`

	// Control receives enable, pause, drain, flush, invalidate, and reset
	// commands.
	Control messaging.Port `akita:"role=github.com/sarchlab/akita/v5/mem/memcontrolprotocol.responder"`
}

// middlewares holds the mmuCache's behavior, run in field order every cycle.
type middlewares struct {
	// Ctrl handles control commands.
	Ctrl *ctrlMiddleware

	// Cache looks up translations, forwards misses, and relays responses.
	Cache *mmuCacheMiddleware
}

// Resources is empty; peer addresses are configured in Spec.
type Resources struct{}

const (
	mmuCacheStateEnable = "enable"
	mmuCacheStatePause  = "pause"
	mmuCacheStateDrain  = "drain"
)

// state contains mutable runtime data for the mmuCache.
type state struct {
	CurrentState    string               `json:"current_state"`
	PendingDrainRsp bool                 `json:"pending_drain_rsp"`
	CurrentCmdID    uint64               `json:"current_cmd_id"`
	CurrentCmdSrc   messaging.RemotePort `json:"current_cmd_src"`
	Table           []setState           `json:"table"`

	// OutstandingBottomReqs holds the IDs of translation requests forwarded
	// to the low module for which no response has returned yet. A Drain is not
	// complete while any are outstanding (otherwise a late response would land
	// after the caller was told the cache had drained), and a response whose
	// ID is absent here is orphaned — e.g. its request was dropped by a Reset
	// issued mid-walk — and must be discarded rather than repopulate the table.
	OutstandingBottomReqs map[uint64]bool `json:"outstanding_bottom_reqs"`

	// InflightReqs records, keyed by the forwarded (Bottom) request ID, the
	// identity of the original Top request that triggered the walk. The
	// tracing completion path only retains message IDs — not the message
	// values — so it reconstructs the original Top request from this state to
	// finalize the req_out and complete the req_in when the Bottom response
	// returns. Entries are added when a walk is forwarded and removed when its
	// response is forwarded upstream (or dropped on Reset).
	InflightReqs map[uint64]inflightReqState `json:"inflight_reqs"`
}

// inflightReqState owns the tracing IDs for a forwarded walk.
type inflightReqState struct {
	RecvTaskID uint64 `json:"recv_task_id"`
	SendTaskID uint64 `json:"send_task_id"`
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
			LRU:    lruset.MakeSet(numBlocks),
		}
		for j := 0; j < numBlocks; j++ {
			s.Blocks[j] = blockState{WayID: j}
		}
		sets[i] = s
	}
	return sets
}

// Comp is the mmuCache component.
type Comp = ticking.Component[Spec, state, Resources, Ports, middlewares]
