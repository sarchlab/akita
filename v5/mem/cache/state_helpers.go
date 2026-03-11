package cache

import (
	"github.com/sarchlab/akita/v5/mem/vm"
	"github.com/sarchlab/akita/v5/sim"
)

// MsgRef is a serializable representation of a sim.Msg's metadata.
type MsgRef struct {
	ID           string         `json:"id"`
	Src          sim.RemotePort `json:"src"`
	Dst          sim.RemotePort `json:"dst"`
	RspTo        string         `json:"rsp_to"`
	TrafficClass string         `json:"traffic_class"`
	TrafficBytes int            `json:"traffic_bytes"`
}

// MsgRefFromMsg converts a sim.Msg into a serializable MsgRef.
func MsgRefFromMsg(msg sim.Msg) MsgRef {
	meta := msg.Meta()
	return MsgRef{
		ID:           meta.ID,
		Src:          meta.Src,
		Dst:          meta.Dst,
		RspTo:        meta.RspTo,
		TrafficClass: meta.TrafficClass,
		TrafficBytes: meta.TrafficBytes,
	}
}

// MsgFromRef converts a MsgRef back into a sim.Msg (as a *sim.GenericMsg).
func MsgFromRef(ref MsgRef) sim.Msg {
	return &sim.GenericMsg{
		MsgMeta: sim.MsgMeta{
			ID:           ref.ID,
			Src:          ref.Src,
			Dst:          ref.Dst,
			RspTo:        ref.RspTo,
			TrafficClass: ref.TrafficClass,
			TrafficBytes: ref.TrafficBytes,
		},
	}
}

// BlockState is a serializable representation of a cache Block.
type BlockState struct {
	PID          uint32 `json:"pid"`
	Tag          uint64 `json:"tag"`
	WayID        int    `json:"way_id"`
	SetID        int    `json:"set_id"`
	CacheAddress uint64 `json:"cache_address"`
	IsValid      bool   `json:"is_valid"`
	IsDirty      bool   `json:"is_dirty"`
	ReadCount    int    `json:"read_count"`
	IsLocked     bool   `json:"is_locked"`
	DirtyMask    []bool `json:"dirty_mask"`
}

// SetState is a serializable representation of a cache Set.
type SetState struct {
	Blocks   []BlockState `json:"blocks"`
	LRUOrder []int        `json:"lru_order"`
}

// DirectoryState is a serializable representation of a DirectoryImpl.
type DirectoryState struct {
	Sets []SetState `json:"sets"`
}

// SnapshotDirectory captures the current state of a Directory into a
// serializable DirectoryState.
func SnapshotDirectory(d Directory) DirectoryState {
	impl := d.(*DirectoryImpl)
	ds := DirectoryState{
		Sets: make([]SetState, len(impl.Sets)),
	}

	for i, set := range impl.Sets {
		ss := SetState{
			Blocks:   make([]BlockState, len(set.Blocks)),
			LRUOrder: make([]int, len(set.LRUQueue)),
		}

		for j, b := range set.Blocks {
			ss.Blocks[j] = snapshotBlock(b)
		}

		for j, b := range set.LRUQueue {
			ss.LRUOrder[j] = b.WayID
		}

		ds.Sets[i] = ss
	}

	return ds
}

func snapshotBlock(b *Block) BlockState {
	bs := BlockState{
		PID:          uint32(b.PID),
		Tag:          b.Tag,
		WayID:        b.WayID,
		SetID:        b.SetID,
		CacheAddress: b.CacheAddress,
		IsValid:      b.IsValid,
		IsDirty:      b.IsDirty,
		ReadCount:    b.ReadCount,
		IsLocked:     b.IsLocked,
	}

	if b.DirtyMask != nil {
		bs.DirtyMask = make([]bool, len(b.DirtyMask))
		copy(bs.DirtyMask, b.DirtyMask)
	}

	return bs
}

// RestoreDirectory restores a Directory from a DirectoryState.
func RestoreDirectory(d Directory, ds DirectoryState) {
	impl := d.(*DirectoryImpl)

	for i, ss := range ds.Sets {
		set := &impl.Sets[i]

		for j, bs := range ss.Blocks {
			restoreBlock(set.Blocks[j], bs)
		}

		set.LRUQueue = make([]*Block, len(ss.LRUOrder))
		for j, wayID := range ss.LRUOrder {
			set.LRUQueue[j] = set.Blocks[wayID]
		}
	}
}

func restoreBlock(b *Block, bs BlockState) {
	b.PID = vm.PID(bs.PID)
	b.Tag = bs.Tag
	b.WayID = bs.WayID
	b.SetID = bs.SetID
	b.CacheAddress = bs.CacheAddress
	b.IsValid = bs.IsValid
	b.IsDirty = bs.IsDirty
	b.ReadCount = bs.ReadCount
	b.IsLocked = bs.IsLocked

	if bs.DirtyMask != nil {
		b.DirtyMask = make([]bool, len(bs.DirtyMask))
		copy(b.DirtyMask, bs.DirtyMask)
	} else {
		b.DirtyMask = nil
	}
}

// MSHREntryState is a serializable representation of an MSHREntry.
type MSHREntryState struct {
	PID                uint32 `json:"pid"`
	Address            uint64 `json:"address"`
	TransactionIndices []int  `json:"transaction_indices"`
	BlockSetID         int    `json:"block_set_id"`
	BlockWayID         int    `json:"block_way_id"`
	HasBlock           bool   `json:"has_block"`
	HasReadReq         bool   `json:"has_read_req"`
	ReadReq            MsgRef `json:"read_req"`
	HasDataReady       bool   `json:"has_data_ready"`
	DataReady          MsgRef `json:"data_ready"`
	Data               []byte `json:"data"`
}

// MSHRState is a serializable representation of an mshrImpl.
type MSHRState struct {
	Entries []MSHREntryState `json:"entries"`
}

// SnapshotMSHR captures the current state of an MSHR into a serializable
// MSHRState. The transLookup map translates each request (interface{}) to
// an integer index so the caller can restore them later.
func SnapshotMSHR(
	m MSHR,
	transLookup map[interface{}]int,
) MSHRState {
	impl := m.(*mshrImpl)
	ms := MSHRState{
		Entries: make([]MSHREntryState, len(impl.entries)),
	}

	for i, e := range impl.entries {
		ms.Entries[i] = snapshotMSHREntry(e, transLookup)
	}

	return ms
}

func snapshotMSHREntry(
	e *MSHREntry,
	transLookup map[interface{}]int,
) MSHREntryState {
	es := MSHREntryState{
		PID:     uint32(e.PID),
		Address: e.Address,
	}

	es.TransactionIndices = make([]int, len(e.Requests))
	for j, req := range e.Requests {
		es.TransactionIndices[j] = transLookup[req]
	}

	if e.Block != nil {
		es.HasBlock = true
		es.BlockSetID = e.Block.SetID
		es.BlockWayID = e.Block.WayID
	}

	if e.ReadReq != nil {
		es.HasReadReq = true
		es.ReadReq = MsgRefFromMsg(e.ReadReq)
	}

	if e.DataReady != nil {
		es.HasDataReady = true
		es.DataReady = MsgRefFromMsg(e.DataReady)
	}

	if e.Data != nil {
		es.Data = make([]byte, len(e.Data))
		copy(es.Data, e.Data)
	}

	return es
}

// RestoreMSHR restores an MSHR from an MSHRState. The transactions slice
// provides the live objects that were previously mapped by SnapshotMSHR's
// transLookup. The dir is used to resolve block references.
func RestoreMSHR(
	m MSHR,
	ms MSHRState,
	transactions []interface{},
	dir Directory,
) {
	impl := m.(*mshrImpl)
	impl.entries = make([]*MSHREntry, len(ms.Entries))

	sets := dir.GetSets()

	for i, es := range ms.Entries {
		impl.entries[i] = restoreMSHREntry(
			es, transactions, sets)
	}
}

func restoreMSHREntry(
	es MSHREntryState,
	transactions []interface{},
	sets []Set,
) *MSHREntry {
	e := NewMSHREntry()
	e.PID = vm.PID(es.PID)
	e.Address = es.Address

	e.Requests = make([]interface{}, len(es.TransactionIndices))
	for j, idx := range es.TransactionIndices {
		e.Requests[j] = transactions[idx]
	}

	if es.HasBlock {
		e.Block = sets[es.BlockSetID].Blocks[es.BlockWayID]
	}

	if es.HasReadReq {
		e.ReadReq = MsgFromRef(es.ReadReq)
	}

	if es.HasDataReady {
		e.DataReady = MsgFromRef(es.DataReady)
	}

	if es.Data != nil {
		e.Data = make([]byte, len(es.Data))
		copy(e.Data, es.Data)
	}

	return e
}
