// Package internal provides the definition required for defining TLB.
package internal

import (
	"fmt"
	"sort"

	"github.com/sarchlab/akita/v3/mem/vm"
)

// A Set holds a certain number of pages.
type Set interface {
	Lookup(pid vm.PID, vAddr uint64) (wayID int, page vm.Page, found bool)
	Update(wayID int, page vm.Page)
	Evict() (wayID int, ok bool)
	Visit(wayID int)
}

// NewSet creates a new TLB set.
func NewSet(numWays int) Set {
	s := &setImpl{}
	s.blocks = make([]*block, numWays)
	s.visitList = make([]*block, 0, numWays)
	s.vAddrWayIDMap = make(map[string]int)
	for i := range s.blocks {
		b := &block{}
		s.blocks[i] = b
		b.wayID = i
		s.Visit(i)
	}
	return s
}

type block struct {
	page      vm.Page
	wayID     int
	lastVisit uint64
}

func (b *block) Less(anotherBlock *block) bool {
	return b.lastVisit < anotherBlock.lastVisit
}

type setImpl struct {
	blocks        []*block
	vAddrWayIDMap map[string]int
	visitList     []*block
	visitCount    uint64
}

func (s *setImpl) keyString(pid vm.PID, vAddr uint64) string {
	return fmt.Sprintf("%d%016x", pid, vAddr)
}

func (s *setImpl) Lookup(pid vm.PID, vAddr uint64) (
	wayID int,
	page vm.Page,
	found bool,
) {
	key := s.keyString(pid, vAddr)
	wayID, ok := s.vAddrWayIDMap[key]
	if !ok {
		return 0, vm.Page{}, false
	}

	block := s.blocks[wayID]

	return block.wayID, block.page, true
}

func (s *setImpl) Update(wayID int, page vm.Page) {
	block := s.blocks[wayID]
	key := s.keyString(block.page.PID, block.page.VAddr)
	delete(s.vAddrWayIDMap, key)

	block.page = page
	key = s.keyString(page.PID, page.VAddr)
	s.vAddrWayIDMap[key] = wayID
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
