package gmmu

import (
	"fmt"
	"log"

	"github.com/sarchlab/akita/v5/mem/vm"
	"github.com/sarchlab/akita/v5/modeling"
	"github.com/sarchlab/akita/v5/sim"
	"github.com/sarchlab/akita/v5/tracing"
)

// walkMW handles the top→page-table walk path:
// parseFromTop, startWalking, walkPageTable, removeCompletedTranslations,
// processRemoteMemReq, finalizePageWalk, doPageWalkHit.
type walkMW struct {
	comp      *modeling.Component[Spec, State]
	pageTable vm.PageTable
}

func (m *walkMW) topPort() sim.Port {
	return m.comp.GetPortByName("Top")
}

func (m *walkMW) bottomPort() sim.Port {
	return m.comp.GetPortByName("Bottom")
}

// Tick runs the walk stages.
func (m *walkMW) Tick() bool {
	madeProgress := false

	madeProgress = m.walkPageTable() || madeProgress
	madeProgress = m.parseFromTop() || madeProgress

	return madeProgress
}

func (m *walkMW) parseFromTop() bool {
	spec := m.comp.GetSpec()
	cur := m.comp.GetState()

	if len(cur.WalkingTranslations) >= spec.MaxRequestsInFlight {
		return false
	}

	reqI := m.topPort().RetrieveIncoming()
	if reqI == nil {
		return false
	}

	switch req := reqI.(type) {
	case *vm.TranslationReq:
		tracing.TraceReqReceive(req, m.comp)
		m.startWalking(req)
	default:
		log.Panicf("gmmu cannot handle request of type %s",
			fmt.Sprintf("%T", reqI))
	}

	return true
}

func (m *walkMW) startWalking(req *vm.TranslationReq) {
	spec := m.comp.GetSpec()
	nextState := m.comp.GetNextState()

	ts := transactionState{
		ReqID:     req.ID,
		ReqSrc:    req.Src,
		ReqDst:    req.Dst,
		PID:       uint64(req.PID),
		VAddr:     req.VAddr,
		DeviceID:  req.DeviceID,
		CycleLeft: spec.Latency,
	}

	nextState.WalkingTranslations = append(
		nextState.WalkingTranslations, ts)
}

func (m *walkMW) walkPageTable() bool {
	cur := m.comp.GetState()

	if len(cur.WalkingTranslations) == 0 {
		return false
	}

	next := m.comp.GetNextState()
	madeProgress := false
	spec := m.comp.GetSpec()

	for i := 0; i < len(cur.WalkingTranslations); i++ {
		if cur.WalkingTranslations[i].CycleLeft > 0 {
			next.WalkingTranslations[i].CycleLeft = cur.WalkingTranslations[i].CycleLeft - 1
			madeProgress = true
			continue
		}

		ts := cur.WalkingTranslations[i]

		page, found := m.pageTable.Find(vm.PID(ts.PID), ts.VAddr)
		if !found {
			log.Panicf(
				"gmmu: page not found for PID %d VAddr 0x%x",
				ts.PID, ts.VAddr,
			)
		}

		if page.DeviceID == spec.DeviceID {
			madeProgress = m.finalizePageWalk(next, i) || madeProgress
		} else {
			madeProgress = m.processRemoteMemReq(next, i) || madeProgress
		}
	}

	m.removeCompletedTranslations(next)

	return madeProgress
}

func (m *walkMW) removeCompletedTranslations(state *State) {
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

func (m *walkMW) processRemoteMemReq(
	state *State,
	walkingIndex int,
) bool {
	if !m.bottomPort().CanSend() {
		return false
	}

	spec := m.comp.GetSpec()
	walking := state.WalkingTranslations[walkingIndex]

	req := &vm.TranslationReq{}
	req.ID = sim.GetIDGenerator().Generate()
	req.Src = m.bottomPort().AsRemote()
	req.Dst = spec.LowModule
	req.PID = vm.PID(walking.PID)
	req.VAddr = walking.VAddr
	req.DeviceID = walking.DeviceID
	req.TrafficClass = "vm.TranslationReq"

	state.RemoteMemReqs[req.ID] = walking

	m.bottomPort().Send(req)

	state.ToRemoveFromPTW = append(state.ToRemoveFromPTW, walkingIndex)

	return true
}

func (m *walkMW) finalizePageWalk(
	state *State,
	walkingIndex int,
) bool {
	ts := state.WalkingTranslations[walkingIndex]
	page, found := m.pageTable.Find(vm.PID(ts.PID), ts.VAddr)
	if !found {
		return false
	}

	state.WalkingTranslations[walkingIndex].Page = pageStateFromPage(page)

	return m.doPageWalkHit(state, walkingIndex)
}

func (m *walkMW) doPageWalkHit(
	state *State,
	walkingIndex int,
) bool {
	if !m.topPort().CanSend() {
		return false
	}
	walking := state.WalkingTranslations[walkingIndex]

	rsp := &vm.TranslationRsp{
		Page: pageFromPageState(walking.Page),
	}
	rsp.ID = sim.GetIDGenerator().Generate()
	rsp.Src = m.topPort().AsRemote()
	rsp.Dst = walking.ReqSrc
	rsp.RspTo = walking.ReqID
	rsp.TrafficClass = "vm.TranslationRsp"

	m.topPort().Send(rsp)

	state.ToRemoveFromPTW = append(state.ToRemoveFromPTW, walkingIndex)

	tracing.TraceReqComplete(
		&vm.TranslationReq{
			MsgMeta: sim.MsgMeta{
				ID:  walking.ReqID,
				Src: walking.ReqSrc,
				Dst: walking.ReqDst,
			},
		},
		m.comp,
	)

	return true
}
