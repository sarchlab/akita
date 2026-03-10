// Package internal provides the definition required for defining mmuCache.
package internal

import (
	"fmt"
	"sort"

	"github.com/sarchlab/akita/v5/mem/vm"
)

// BlockState is a serializable snapshot of a single cache block.
type BlockState struct {
	PID       uint64 `json:"pid"`
	Seg       uint64 `json:"seg"`
	WayID     int    `json:"way_id"`
	LastVisit uint64 `json:"last_visit"`
}

// SetState is a serializable snapshot of a Set.
type SetState struct {
	Blocks      []BlockState   `json:"blocks"`
	SegWayIDMap map[string]int `json:"seg_way_id_map"`
	VisitOrder  []int          `json:"visit_order"`
	VisitCount  uint64         `json:"visit_count"`
}

// A Set holds a certain number of pages.
type Set interface {
	Lookup(pid vm.PID, seg uint64) (wayID int, found bool)
	Update(wayID int, pid vm.PID, seg uint64)
	Evict() (wayID int, ok bool)
	Visit(wayID int)
	ExportState() SetState
	ImportState(state SetState)
}

// NewSet creates a new mmuCache set.
func NewSet(numWays int) Set {
	s := &setImpl{}
	s.blocks = make([]*block, numWays)
	s.visitList = make([]*block, 0, numWays)
	s.segWayIDMap = make(map[string]int)
	for i := range s.blocks {
		b := &block{}
		s.blocks[i] = b
		b.wayID = i
		s.Visit(i)
	}
	return s
}

type block struct {
	PID       vm.PID
	seg       uint64
	wayID     int
	lastVisit uint64
}

func (b *block) Less(anotherBlock *block) bool {
	return b.lastVisit < anotherBlock.lastVisit
}

type setImpl struct {
	blocks      []*block
	segWayIDMap map[string]int
	visitList   []*block
	visitCount  uint64
}

func (s *setImpl) keyString(pid vm.PID, seg uint64) string {
	return fmt.Sprintf("%d%016x", pid, seg)
}

func (s *setImpl) Lookup(pid vm.PID, seg uint64) (
	wayID int,
	found bool,
) {
	key := s.keyString(pid, seg)
	wayID, ok := s.segWayIDMap[key]
	if !ok {
		return 0, false
	}

	block := s.blocks[wayID]

	return block.wayID, true
}

func (s *setImpl) Update(wayID int, pid vm.PID, seg uint64) {
	block := s.blocks[wayID]
	key := s.keyString(block.PID, block.seg)
	delete(s.segWayIDMap, key)

	block.PID = pid
	block.seg = seg
	key = s.keyString(pid, seg)
	s.segWayIDMap[key] = wayID
}

func (s *setImpl) Evict() (wayID int, ok bool) {
	if s.hasNothingToEvict() {
		return 0, false
	}

	leastVisited := s.visitList[0]
	wayID = leastVisited.wayID
	s.visitList = s.visitList[1:]
	return wayID, true
}

func (s *setImpl) Visit(wayID int) {
	block := s.blocks[wayID]

	for i, b := range s.visitList {
		if b.wayID == wayID {
			s.visitList = append(s.visitList[:i], s.visitList[i+1:]...)
			break
		}
	}

	s.visitCount++
	block.lastVisit = s.visitCount

	index := sort.Search(len(s.visitList), func(i int) bool {
		return s.visitList[i].lastVisit > block.lastVisit
	})

	s.visitList = append(s.visitList, nil)
	copy(s.visitList[index+1:], s.visitList[index:])
	s.visitList[index] = block
}

func (s *setImpl) hasNothingToEvict() bool {
	return len(s.visitList) == 0
}

// ExportState returns a serializable snapshot of the set.
func (s *setImpl) ExportState() SetState {
	ss := SetState{
		Blocks:      make([]BlockState, len(s.blocks)),
		SegWayIDMap: make(map[string]int, len(s.segWayIDMap)),
		VisitOrder:  make([]int, len(s.visitList)),
		VisitCount:  s.visitCount,
	}

	for i, b := range s.blocks {
		ss.Blocks[i] = BlockState{
			PID:       uint64(b.PID),
			Seg:       b.seg,
			WayID:     b.wayID,
			LastVisit: b.lastVisit,
		}
	}

	for k, v := range s.segWayIDMap {
		ss.SegWayIDMap[k] = v
	}

	for i, b := range s.visitList {
		ss.VisitOrder[i] = b.wayID
	}

	return ss
}

// ImportState restores the set from a serializable snapshot.
func (s *setImpl) ImportState(ss SetState) {
	s.blocks = make([]*block, len(ss.Blocks))
	for i, bs := range ss.Blocks {
		s.blocks[i] = &block{
			PID:       vm.PID(bs.PID),
			seg:       bs.Seg,
			wayID:     bs.WayID,
			lastVisit: bs.LastVisit,
		}
	}

	s.segWayIDMap = make(map[string]int, len(ss.SegWayIDMap))
	for k, v := range ss.SegWayIDMap {
		s.segWayIDMap[k] = v
	}

	s.visitCount = ss.VisitCount

	// Rebuild visitList from VisitOrder, which stores wayIDs in LRU order.
	s.visitList = make([]*block, len(ss.VisitOrder))
	for i, wayID := range ss.VisitOrder {
		s.visitList[i] = s.blocks[wayID]
	}
}
