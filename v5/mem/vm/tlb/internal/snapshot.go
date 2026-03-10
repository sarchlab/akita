package internal

import "github.com/sarchlab/akita/v5/mem/vm"

// BlockSnapshot captures the state of a single block.
type BlockSnapshot struct {
	Page      vm.Page `json:"page"`
	WayID     int     `json:"way_id"`
	LastVisit uint64  `json:"last_visit"`
}

// SetSnapshot captures the full serializable state of a Set.
type SetSnapshot struct {
	Blocks     []BlockSnapshot `json:"blocks"`
	VisitOrder []int           `json:"visit_order"`
	VisitCount uint64          `json:"visit_count"`
}

// SnapshotSet extracts the serializable state from a Set.
func SnapshotSet(s Set) SetSnapshot {
	impl := s.(*setImpl)
	snap := SetSnapshot{
		Blocks:     make([]BlockSnapshot, len(impl.blocks)),
		VisitCount: impl.visitCount,
	}
	for i, b := range impl.blocks {
		snap.Blocks[i] = BlockSnapshot{
			Page:      b.page,
			WayID:     b.wayID,
			LastVisit: b.lastVisit,
		}
	}
	snap.VisitOrder = make([]int, len(impl.visitList))
	for i, b := range impl.visitList {
		snap.VisitOrder[i] = b.wayID
	}
	return snap
}

// RestoreSet restores a Set from a snapshot.
func RestoreSet(s Set, snap SetSnapshot) {
	impl := s.(*setImpl)
	impl.visitCount = snap.VisitCount
	impl.vAddrWayIDMap = make(map[string]int)
	for i, bs := range snap.Blocks {
		impl.blocks[i].page = bs.Page
		impl.blocks[i].wayID = bs.WayID
		impl.blocks[i].lastVisit = bs.LastVisit
		if bs.Page.Valid {
			key := impl.keyString(bs.Page.PID, bs.Page.VAddr)
			impl.vAddrWayIDMap[key] = bs.WayID
		}
	}
	// Rebuild visitList from VisitOrder
	impl.visitList = make([]*block, len(snap.VisitOrder))
	for i, wayID := range snap.VisitOrder {
		impl.visitList[i] = impl.blocks[wayID]
	}
}
