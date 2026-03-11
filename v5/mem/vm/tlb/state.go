package tlb

import (
	"io"

	"github.com/sarchlab/akita/v5/mem/vm"
	"github.com/sarchlab/akita/v5/mem/vm/tlb/internal"
	"github.com/sarchlab/akita/v5/queueing"
	"github.com/sarchlab/akita/v5/sim"
)

// msgRef is a serializable representation of a message's metadata.
type msgRef struct {
	ID           string         `json:"id"`
	Src          sim.RemotePort `json:"src"`
	Dst          sim.RemotePort `json:"dst"`
	RspTo        string         `json:"rsp_to"`
	TrafficClass string         `json:"traffic_class"`
	TrafficBytes int            `json:"traffic_bytes"`
}

// blockState is a serializable representation of an internal block.
type blockState struct {
	Page      vm.Page `json:"page"`
	WayID     int     `json:"way_id"`
	LastVisit uint64  `json:"last_visit"`
}

// setState is the serializable representation of one TLB set.
type setState struct {
	Blocks     []blockState `json:"blocks"`
	VisitList  []int        `json:"visit_list"`
	VisitCount uint64       `json:"visit_count"`
}

// mshrEntryState is a serializable representation of an mshrEntry.
type mshrEntryState struct {
	PID            uint32   `json:"pid"`
	VAddr          uint64   `json:"vaddr"`
	Requests       []msgRef `json:"requests"`
	HasReqToBottom bool     `json:"has_req_to_bottom"`
	ReqToBottom    msgRef   `json:"req_to_bottom"`
	Page           vm.Page  `json:"page"`
}

// pipelineTLBReqState is a serializable pipeline item.
type pipelineTLBReqState struct {
	Msg msgRef `json:"msg"`
}

// pipelineStageState captures one non-nil pipeline slot.
type pipelineStageState struct {
	Lane      int                 `json:"lane"`
	Stage     int                 `json:"stage"`
	Item      pipelineTLBReqState `json:"item"`
	CycleLeft int                 `json:"cycle_left"`
}

func msgRefFromTranslationReq(msg *vm.TranslationReq) msgRef {
	return msgRef{
		ID:           msg.ID,
		Src:          msg.Src,
		Dst:          msg.Dst,
		RspTo:        msg.RspTo,
		TrafficClass: msg.TrafficClass,
		TrafficBytes: msg.TrafficBytes,
	}
}

func translationReqFromRef(ref msgRef) *vm.TranslationReq {
	r := &vm.TranslationReq{}
	r.ID = ref.ID
	r.Src = ref.Src
	r.Dst = ref.Dst
	r.RspTo = ref.RspTo
	r.TrafficClass = ref.TrafficClass
	r.TrafficBytes = ref.TrafficBytes
	return r
}

func msgRefFromFlushReq(msg *FlushReq) msgRef {
	return msgRef{
		ID:           msg.ID,
		Src:          msg.Src,
		Dst:          msg.Dst,
		RspTo:        msg.RspTo,
		TrafficClass: msg.TrafficClass,
		TrafficBytes: msg.TrafficBytes,
	}
}

func flushReqFromRef(ref msgRef) *FlushReq {
	r := &FlushReq{}
	r.ID = ref.ID
	r.Src = ref.Src
	r.Dst = ref.Dst
	r.RspTo = ref.RspTo
	r.TrafficClass = ref.TrafficClass
	r.TrafficBytes = ref.TrafficBytes
	return r
}

func snapshotMSHR(m mshr) []mshrEntryState {
	impl := m.(*mshrImpl)
	states := make([]mshrEntryState, len(impl.entries))
	for i, e := range impl.entries {
		states[i] = mshrEntryState{
			PID:   uint32(e.pid),
			VAddr: e.vAddr,
			Page:  e.page,
		}
		states[i].Requests = make([]msgRef, len(e.Requests))
		for j, r := range e.Requests {
			states[i].Requests[j] = msgRefFromTranslationReq(r)
		}
		if e.reqToBottom != nil {
			states[i].HasReqToBottom = true
			states[i].ReqToBottom = msgRefFromTranslationReq(e.reqToBottom)
		}
	}
	return states
}

func restoreMSHR(m mshr, states []mshrEntryState) {
	impl := m.(*mshrImpl)
	impl.entries = make([]*mshrEntry, len(states))
	for i, s := range states {
		entry := &mshrEntry{
			pid:   vm.PID(s.PID),
			vAddr: s.VAddr,
			page:  s.Page,
		}
		entry.Requests = make([]*vm.TranslationReq, len(s.Requests))
		for j, r := range s.Requests {
			entry.Requests[j] = translationReqFromRef(r)
		}
		if s.HasReqToBottom {
			entry.reqToBottom = translationReqFromRef(s.ReqToBottom)
		}
		impl.entries[i] = entry
	}
}

func snapshotSets(sets []internal.Set) []setState {
	states := make([]setState, len(sets))
	for i, s := range sets {
		snap := internal.SnapshotSet(s)
		ss := setState{
			Blocks:     make([]blockState, len(snap.Blocks)),
			VisitList:  snap.VisitOrder,
			VisitCount: snap.VisitCount,
		}
		for j, b := range snap.Blocks {
			ss.Blocks[j] = blockState{
				Page:      b.Page,
				WayID:     b.WayID,
				LastVisit: b.LastVisit,
			}
		}
		states[i] = ss
	}
	return states
}

func restoreSets(sets []internal.Set, states []setState) {
	for i, ss := range states {
		snap := internal.SetSnapshot{
			Blocks:     make([]internal.BlockSnapshot, len(ss.Blocks)),
			VisitOrder: ss.VisitList,
			VisitCount: ss.VisitCount,
		}
		for j, b := range ss.Blocks {
			snap.Blocks[j] = internal.BlockSnapshot{
				Page:      b.Page,
				WayID:     b.WayID,
				LastVisit: b.LastVisit,
			}
		}
		internal.RestoreSet(sets[i], snap)
	}
}

func snapshotPipeline(p queueing.Pipeline) []pipelineStageState {
	snaps := queueing.SnapshotPipeline(p)
	states := make([]pipelineStageState, len(snaps))
	for i, s := range snaps {
		req := s.Elem.(*pipelineTLBReq)
		states[i] = pipelineStageState{
			Lane:  s.Lane,
			Stage: s.Stage,
			Item: pipelineTLBReqState{
				Msg: msgRefFromTranslationReq(req.msg),
			},
			CycleLeft: s.CycleLeft,
		}
	}
	return states
}

func restorePipeline(p queueing.Pipeline, states []pipelineStageState) {
	snaps := make([]queueing.PipelineStageSnapshot, len(states))
	for i, s := range states {
		snaps[i] = queueing.PipelineStageSnapshot{
			Lane:  s.Lane,
			Stage: s.Stage,
			Elem: &pipelineTLBReq{
				msg: translationReqFromRef(s.Item.Msg),
			},
			CycleLeft: s.CycleLeft,
		}
	}
	queueing.RestorePipeline(p, snaps)
}

func snapshotBuffer(b queueing.Buffer) []pipelineTLBReqState {
	elems := queueing.SnapshotBuffer(b)
	states := make([]pipelineTLBReqState, len(elems))
	for i, e := range elems {
		req := e.(*pipelineTLBReq)
		states[i] = pipelineTLBReqState{
			Msg: msgRefFromTranslationReq(req.msg),
		}
	}
	return states
}

func restoreBuffer(b queueing.Buffer, states []pipelineTLBReqState) {
	elems := make([]interface{}, len(states))
	for i, s := range states {
		elems[i] = &pipelineTLBReq{
			msg: translationReqFromRef(s.Msg),
		}
	}
	queueing.RestoreBuffer(b, elems)
}

func snapshotMSHREntry(e *mshrEntry) mshrEntryState {
	s := mshrEntryState{
		PID:   uint32(e.pid),
		VAddr: e.vAddr,
		Page:  e.page,
	}
	s.Requests = make([]msgRef, len(e.Requests))
	for j, r := range e.Requests {
		s.Requests[j] = msgRefFromTranslationReq(r)
	}
	if e.reqToBottom != nil {
		s.HasReqToBottom = true
		s.ReqToBottom = msgRefFromTranslationReq(e.reqToBottom)
	}
	return s
}

func restoreMSHREntry(s mshrEntryState) *mshrEntry {
	entry := &mshrEntry{
		pid:   vm.PID(s.PID),
		vAddr: s.VAddr,
		page:  s.Page,
	}
	entry.Requests = make([]*vm.TranslationReq, len(s.Requests))
	for j, r := range s.Requests {
		entry.Requests[j] = translationReqFromRef(r)
	}
	if s.HasReqToBottom {
		entry.reqToBottom = translationReqFromRef(s.ReqToBottom)
	}
	return entry
}

// snapshotState converts runtime mutable data into a serializable State.
func (c *Comp) snapshotState() State {
	s := State{
		TLBState: c.state,
	}

	s.Sets = snapshotSets(c.sets)
	s.MSHREntries = snapshotMSHR(c.mshr)

	if c.respondingMSHREntry != nil {
		s.HasRespondingMSHR = true
		s.RespondingMSHRData = snapshotMSHREntry(c.respondingMSHREntry)
	}

	s.PipelineStages = snapshotPipeline(c.responsePipeline)
	s.BufferItems = snapshotBuffer(c.responseBuffer)

	if c.inflightFlushReq != nil {
		s.HasInflightFlushReq = true
		s.InflightFlushReqMsg = msgRefFromFlushReq(c.inflightFlushReq)
	}

	return s
}

// restoreFromState restores runtime mutable data from a serializable State.
func (c *Comp) restoreFromState(s State) {
	c.state = s.TLBState

	restoreSets(c.sets, s.Sets)
	restoreMSHR(c.mshr, s.MSHREntries)

	if s.HasRespondingMSHR {
		c.respondingMSHREntry = restoreMSHREntry(s.RespondingMSHRData)
	} else {
		c.respondingMSHREntry = nil
	}

	restorePipeline(c.responsePipeline, s.PipelineStages)
	restoreBuffer(c.responseBuffer, s.BufferItems)

	if s.HasInflightFlushReq {
		c.inflightFlushReq = flushReqFromRef(s.InflightFlushReqMsg)
	} else {
		c.inflightFlushReq = nil
	}
}

// GetState converts runtime mutable data into a serializable State.
func (c *Comp) GetState() State {
	state := c.snapshotState()
	c.Component.SetState(state)
	return state
}

// SetState restores runtime mutable data from a serializable State.
func (c *Comp) SetState(state State) {
	c.Component.SetState(state)
	c.restoreFromState(state)
}

// SaveState marshals the component's spec and state as JSON, ensuring the
// runtime fields are synced into State first.
func (c *Comp) SaveState(w io.Writer) error {
	c.GetState()
	return c.Component.SaveState(w)
}

// LoadState reads JSON from r and restores both the base state and the
// runtime fields.
func (c *Comp) LoadState(r io.Reader) error {
	if err := c.Component.LoadState(r); err != nil {
		return err
	}
	c.SetState(c.Component.GetState())
	return nil
}

// SyncState copies mutable runtime data into the State struct.
// Deprecated: Use GetState() instead.
func (c *Comp) SyncState() {
	c.GetState()
}
