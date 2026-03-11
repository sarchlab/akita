package mmuCache

import (
	"fmt"
	"sort"

	"github.com/sarchlab/akita/v5/mem/vm"
)

// blockState is a serializable snapshot of a single cache block.
type blockState struct {
	PID       uint64 `json:"pid"`
	Seg       uint64 `json:"seg"`
	WayID     int    `json:"way_id"`
	LastVisit uint64 `json:"last_visit"`
}

// setState is the serializable snapshot of one mmuCache set (level).
type setState struct {
	Blocks      []blockState   `json:"blocks"`
	SegWayIDMap map[string]int `json:"seg_way_id_map"`
	VisitOrder  []int          `json:"visit_order"`
	VisitCount  uint64         `json:"visit_count"`
}

// --- Free functions for Set operations ---

func setKeyString(pid vm.PID, seg uint64) string {
	return fmt.Sprintf("%d%016x", pid, seg)
}

func setLookup(s *setState, pid vm.PID, seg uint64) (wayID int, found bool) {
	key := setKeyString(pid, seg)
	wayID, ok := s.SegWayIDMap[key]
	if !ok {
		return 0, false
	}
	return s.Blocks[wayID].WayID, true
}

func setUpdate(s *setState, wayID int, pid vm.PID, seg uint64) {
	block := &s.Blocks[wayID]
	// Remove old mapping
	oldKey := setKeyString(vm.PID(block.PID), block.Seg)
	delete(s.SegWayIDMap, oldKey)
	// Update block
	block.PID = uint64(pid)
	block.Seg = seg
	// Add new mapping
	newKey := setKeyString(pid, seg)
	s.SegWayIDMap[newKey] = wayID
}

func setEvict(s *setState) (wayID int, ok bool) {
	if len(s.VisitOrder) == 0 {
		return 0, false
	}
	wayID = s.VisitOrder[0]
	s.VisitOrder = s.VisitOrder[1:]
	return wayID, true
}

func setVisit(s *setState, wayID int) {
	// Remove wayID from visit order if present
	for i, w := range s.VisitOrder {
		if w == wayID {
			s.VisitOrder = append(s.VisitOrder[:i], s.VisitOrder[i+1:]...)
			break
		}
	}

	s.VisitCount++
	s.Blocks[wayID].LastVisit = s.VisitCount

	// Insert in sorted position by lastVisit
	targetVisit := s.VisitCount
	index := sort.Search(len(s.VisitOrder), func(i int) bool {
		return s.Blocks[s.VisitOrder[i]].LastVisit > targetVisit
	})

	s.VisitOrder = append(s.VisitOrder, 0)
	copy(s.VisitOrder[index+1:], s.VisitOrder[index:])
	s.VisitOrder[index] = wayID
}

func initSets(numLevels, numBlocks int) []setState {
	sets := make([]setState, numLevels)
	for i := 0; i < numLevels; i++ {
		s := setState{
			Blocks:      make([]blockState, numBlocks),
			SegWayIDMap: make(map[string]int),
		}
		for j := 0; j < numBlocks; j++ {
			s.Blocks[j] = blockState{WayID: j}
		}
		// Visit each way to populate VisitOrder (LRU initialization)
		s.VisitOrder = make([]int, 0, numBlocks)
		for j := 0; j < numBlocks; j++ {
			setVisit(&s, j)
		}
		sets[i] = s
	}
	return sets
}
