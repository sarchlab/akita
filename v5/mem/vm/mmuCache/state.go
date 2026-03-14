package mmuCache

import (
	"github.com/sarchlab/akita/v5/mem/vm"
	"github.com/sarchlab/akita/v5/mem/vm/lruset"
	"github.com/sarchlab/akita/v5/sim"
)

const (
	mmuCacheStateEnable = "enable"
	mmuCacheStatePause  = "pause"
	mmuCacheStateDrain  = "drain"
	mmuCacheStateFlush  = "flush"
)

// State contains mutable runtime data for the mmuCache.
type State struct {
	CurrentState           string         `json:"current_state"`
	Table                  []setState     `json:"table"`
	InflightFlushReqID     string         `json:"inflight_flush_req_id"`
	InflightFlushReqSrc    sim.RemotePort `json:"inflight_flush_req_src"`
	InflightFlushReqActive bool           `json:"inflight_flush_req_active"`
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
	wayID, ok := lruset.Lookup(&s.LRU, key)
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
	lruset.UpdateKey(&s.LRU, wayID, oldKey, newKey)
}

func setEvict(s *setState) (wayID int, ok bool) {
	return lruset.Evict(&s.LRU)
}

func setVisit(s *setState, wayID int) {
	lruset.Visit(&s.LRU, wayID)
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
