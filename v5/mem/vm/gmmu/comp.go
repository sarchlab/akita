package gmmu

import (
	"fmt"
	"log"

	"github.com/sarchlab/akita/v5/mem/vm"
	"github.com/sarchlab/akita/v5/modeling"
	"github.com/sarchlab/akita/v5/sim"
	"github.com/sarchlab/akita/v5/tracing"
)

// Spec contains immutable configuration for the GMMU.
type Spec struct {
	DeviceID            uint64         `json:"device_id"`
	Log2PageSize        uint64         `json:"log2_page_size"`
	Latency             int            `json:"latency"`
	MaxRequestsInFlight int            `json:"max_requests_in_flight"`
	LowModule           sim.RemotePort `json:"low_module"`
}

// pageState captures vm.Page fields in a serializable form.
type pageState struct {
	PID         uint64 `json:"pid"`
	VAddr       uint64 `json:"vaddr"`
	PAddr       uint64 `json:"paddr"`
	PageSize    uint64 `json:"page_size"`
	Valid       bool   `json:"valid"`
	DeviceID    uint64 `json:"device_id"`
	Unified     bool   `json:"unified"`
	IsMigrating bool   `json:"is_migrating"`
	IsPinned    bool   `json:"is_pinned"`
}

// transactionState is the serializable form of a runtime transaction.
type transactionState struct {
	ReqID     string         `json:"req_id"`
	ReqSrc    sim.RemotePort `json:"req_src"`
	ReqDst    sim.RemotePort `json:"req_dst"`
	PID       uint64         `json:"pid"`
	VAddr     uint64         `json:"vaddr"`
	DeviceID  uint64         `json:"device_id"`
	Page      pageState      `json:"page"`
	CycleLeft int            `json:"cycle_left"`
}

// devicePageAccess records pages accessed by a single device.
type devicePageAccess struct {
	DeviceID   uint64   `json:"device_id"`
	PageVAddrs []uint64 `json:"page_vaddrs"`
}

// State contains mutable runtime data for the GMMU.
type State struct {
	WalkingTranslations    []transactionState         `json:"walking_translations"`
	RemoteMemReqs          map[string]transactionState `json:"remote_mem_reqs"`
	ToRemoveFromPTW        []int                      `json:"to_remove_from_ptw"`
	PageAccessedByDeviceID []devicePageAccess          `json:"page_accessed_by_device_id"`
}

// GMMU is the default gmmu implementation. It is also an akita Component.
type GMMU struct {
	*modeling.Component[Spec, State]

	topPort    sim.Port
	bottomPort sim.Port

	// MigrationServiceProvider is the port used for page migration requests and
	// responses between the GMMU and the migration service.
	MigrationServiceProvider sim.RemotePort

	pageTable vm.PageTable
}

// gmmuMiddleware provides the Tick method for the GMMU.
type gmmuMiddleware struct {
	*GMMU
}

// Tick defines how the gmmu updates state each cycle.
func (m *gmmuMiddleware) Tick() bool {
	madeProgress := false

	madeProgress = m.walkPageTable() || madeProgress
	madeProgress = m.parseFromTop() || madeProgress
	madeProgress = m.fetchFromBottom() || madeProgress

	return madeProgress
}

func (gmmu *GMMU) parseFromTop() bool {
	spec := gmmu.GetSpec()
	state := gmmu.GetState()

	if len(state.WalkingTranslations) >= spec.MaxRequestsInFlight {
		return false
	}

	reqI := gmmu.topPort.RetrieveIncoming()
	if reqI == nil {
		return false
	}

	switch req := reqI.(type) {
	case *vm.TranslationReq:
		tracing.TraceReqReceive(req, gmmu)
		gmmu.startWalking(req)
	default:
		log.Panicf("gmmu cannot handle request of type %s", fmt.Sprintf("%T", reqI))
	}

	return true
}

func (gmmu *GMMU) startWalking(req *vm.TranslationReq) {
	spec := gmmu.GetSpec()
	state := gmmu.GetState()

	ts := transactionState{
		ReqID:     req.ID,
		ReqSrc:    req.Src,
		ReqDst:    req.Dst,
		PID:       uint64(req.PID),
		VAddr:     req.VAddr,
		DeviceID:  req.DeviceID,
		CycleLeft: spec.Latency,
	}

	state.WalkingTranslations = append(state.WalkingTranslations, ts)
	gmmu.SetState(state)
}

func (gmmu *GMMU) walkPageTable() bool {
	state := gmmu.GetState()

	if len(state.WalkingTranslations) == 0 {
		return false
	}

	madeProgress := false
	spec := gmmu.GetSpec()

	for i := 0; i < len(state.WalkingTranslations); i++ {
		if state.WalkingTranslations[i].CycleLeft > 0 {
			state.WalkingTranslations[i].CycleLeft--
			madeProgress = true
			continue
		}

		ts := state.WalkingTranslations[i]

		page, found := gmmu.pageTable.Find(vm.PID(ts.PID), ts.VAddr)
		if !found {
			log.Panicf(
				"gmmu: page not found for PID %d VAddr 0x%x",
				ts.PID, ts.VAddr,
			)
		}

		if page.DeviceID == spec.DeviceID {
			madeProgress = gmmu.finalizePageWalk(&state, i) || madeProgress
		} else {
			madeProgress = gmmu.processRemoteMemReq(&state, i) || madeProgress
		}
	}

	gmmu.removeCompletedTranslations(&state)
	gmmu.SetState(state)

	return madeProgress
}

func (gmmu *GMMU) removeCompletedTranslations(state *State) {
	if len(state.ToRemoveFromPTW) == 0 {
		return
	}

	toRemoveSet := make(map[int]bool, len(state.ToRemoveFromPTW))
	for _, idx := range state.ToRemoveFromPTW {
		toRemoveSet[idx] = true
	}

	tmp := state.WalkingTranslations[:0]
	for i := 0; i < len(state.WalkingTranslations); i++ {
		if !toRemoveSet[i] {
			tmp = append(tmp, state.WalkingTranslations[i])
		}
	}
	state.WalkingTranslations = tmp
	state.ToRemoveFromPTW = nil
}

func (gmmu *GMMU) processRemoteMemReq(state *State, walkingIndex int) bool {
	if !gmmu.bottomPort.CanSend() {
		return false
	}

	spec := gmmu.GetSpec()
	walking := state.WalkingTranslations[walkingIndex]

	req := &vm.TranslationReq{}
	req.ID = sim.GetIDGenerator().Generate()
	req.Src = gmmu.bottomPort.AsRemote()
	req.Dst = spec.LowModule
	req.PID = vm.PID(walking.PID)
	req.VAddr = walking.VAddr
	req.DeviceID = walking.DeviceID
	req.TrafficClass = "vm.TranslationReq"

	state.RemoteMemReqs[req.ID] = walking

	gmmu.bottomPort.Send(req)

	state.ToRemoveFromPTW = append(state.ToRemoveFromPTW, walkingIndex)

	return true
}

func (gmmu *GMMU) finalizePageWalk(
	state *State,
	walkingIndex int,
) bool {
	ts := state.WalkingTranslations[walkingIndex]
	page, found := gmmu.pageTable.Find(vm.PID(ts.PID), ts.VAddr)
	if !found {
		return false
	}

	state.WalkingTranslations[walkingIndex].Page = pageStateFromPage(page)

	return gmmu.doPageWalkHit(state, walkingIndex)
}

func (gmmu *GMMU) doPageWalkHit(
	state *State,
	walkingIndex int,
) bool {
	if !gmmu.topPort.CanSend() {
		return false
	}
	walking := state.WalkingTranslations[walkingIndex]

	rsp := &vm.TranslationRsp{
		Page: pageFromPageState(walking.Page),
	}
	rsp.ID = sim.GetIDGenerator().Generate()
	rsp.Src = gmmu.topPort.AsRemote()
	rsp.Dst = walking.ReqSrc
	rsp.RspTo = walking.ReqID
	rsp.TrafficClass = "vm.TranslationRsp"

	gmmu.topPort.Send(rsp)

	state.ToRemoveFromPTW = append(state.ToRemoveFromPTW, walkingIndex)

	tracing.TraceReqComplete(
		&vm.TranslationReq{
			MsgMeta: sim.MsgMeta{
				ID:  walking.ReqID,
				Src: walking.ReqSrc,
				Dst: walking.ReqDst,
			},
		},
		gmmu,
	)

	return true
}

func (gmmu *GMMU) fetchFromBottom() bool {
	if !gmmu.topPort.CanSend() {
		return false
	}

	rspI := gmmu.bottomPort.RetrieveIncoming()
	if rspI == nil {
		return false
	}

	switch rsp := rspI.(type) {
	case *vm.TranslationRsp:
		tracing.TraceReqReceive(rsp, gmmu)
		return gmmu.handleTranslationRsp(rsp)
	default:
		log.Panicf("gmmu cannot handle request of type %s", fmt.Sprintf("%T", rspI))
		return false
	}
}

func (gmmu *GMMU) handleTranslationRsp(rsp *vm.TranslationRsp) bool {
	state := gmmu.GetState()

	reqTransaction, exists := state.RemoteMemReqs[rsp.RspTo]

	if !exists || reqTransaction.ReqID == "" {
		log.Panicf("Cannot find matching request for response %+v", rsp)
	}

	if !gmmu.topPort.CanSend() {
		return false
	}

	rspToTop := &vm.TranslationRsp{
		Page: rsp.Page,
	}
	rspToTop.ID = sim.GetIDGenerator().Generate()
	rspToTop.Src = gmmu.topPort.AsRemote()
	rspToTop.Dst = reqTransaction.ReqSrc
	rspToTop.RspTo = rsp.ID
	rspToTop.TrafficClass = "vm.TranslationRsp"

	gmmu.topPort.Send(rspToTop)

	delete(state.RemoteMemReqs, rsp.RspTo)
	gmmu.SetState(state)

	return true
}

// pageStateFromPage converts a vm.Page to a serializable pageState.
func pageStateFromPage(p vm.Page) pageState {
	return pageState{
		PID:         uint64(p.PID),
		VAddr:       p.VAddr,
		PAddr:       p.PAddr,
		PageSize:    p.PageSize,
		Valid:       p.Valid,
		DeviceID:    p.DeviceID,
		Unified:     p.Unified,
		IsMigrating: p.IsMigrating,
		IsPinned:    p.IsPinned,
	}
}

// pageFromPageState converts a pageState back to a vm.Page.
func pageFromPageState(ps pageState) vm.Page {
	return vm.Page{
		PID:         vm.PID(ps.PID),
		VAddr:       ps.VAddr,
		PAddr:       ps.PAddr,
		PageSize:    ps.PageSize,
		Valid:       ps.Valid,
		DeviceID:    ps.DeviceID,
		Unified:     ps.Unified,
		IsMigrating: ps.IsMigrating,
		IsPinned:    ps.IsPinned,
	}
}
