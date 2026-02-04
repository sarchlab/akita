// Package internal provides the definition required for defining TLB.
package internal

import (
	"fmt"
	"sort"

	"github.com/sarchlab/akita/v4/mem/vm"
)

// A Set holds a certain number of pages.
type Set interface {
	Lookup(pid vm.PID, seg uint64) (wayID int, found bool)
	Update(wayID int, pid vm.PID, seg uint64)
	Evict() (wayID int, ok bool)
	Visit(wayID int)
}

// NewSet creates a new TLB set.
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

	// wayID = s.visitTree.DeleteMin().(*block).wayID
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
