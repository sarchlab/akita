package gmmu

import (
	"log"
	"reflect"

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

// State contains mutable runtime data for the GMMU.
// Runtime data with pointers/interfaces stays on the GMMU struct.
type State struct{}

type transaction struct {
	req        *sim.Msg // payload: *vm.TranslationReqPayload
	reqPayload *vm.TranslationReqPayload
	page       vm.Page
	cycleLeft  int
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

	walkingTranslations []transaction
	remoteMemReqs       map[string]transaction

	toRemoveFromPTW []int

	// PageAccessedByDeviceID records, for each device ID, the pages that have
	// been accessed.
	PageAccessedByDeviceID map[uint64][]uint64
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
	if len(gmmu.walkingTranslations) >= spec.MaxRequestsInFlight {
		return false
	}

	req := gmmu.topPort.RetrieveIncoming()
	if req == nil {
		return false
	}

	tracing.TraceReqReceive(req, gmmu)

	switch req.Payload.(type) {
	case *vm.TranslationReqPayload:
		gmmu.startWalking(req)
	default:
		log.Panicf("gmmu cannot handle request of type %s", reflect.TypeOf(req.Payload))
	}

	return true
}

func (gmmu *GMMU) startWalking(req *sim.Msg) {
	spec := gmmu.GetSpec()
	payload := sim.MsgPayload[vm.TranslationReqPayload](req)
	translationInPipeline := transaction{
		req:        req,
		reqPayload: payload,
		cycleLeft:  spec.Latency,
	}

	gmmu.walkingTranslations = append(gmmu.walkingTranslations, translationInPipeline)
}

func (gmmu *GMMU) walkPageTable() bool {
	madeProgress := false

	if len(gmmu.walkingTranslations) == 0 {
		return false
	}

	spec := gmmu.GetSpec()

	for i := 0; i < len(gmmu.walkingTranslations); i++ {
		if gmmu.walkingTranslations[i].cycleLeft > 0 {
			gmmu.walkingTranslations[i].cycleLeft--
			madeProgress = true
			continue
		}
		payload := gmmu.walkingTranslations[i].reqPayload

		page, found := gmmu.pageTable.Find(payload.PID, payload.VAddr)
		if !found {
			log.Panicf(
				"gmmu: page not found for PID %d VAddr 0x%x",
				payload.PID, payload.VAddr,
			)
		}

		if page.DeviceID == spec.DeviceID {
			madeProgress = gmmu.finalizePageWalk(i) || madeProgress
		} else {
			madeProgress = gmmu.processRemoteMemReq(i) || madeProgress
		}
	}

	gmmu.removeCompletedTranslations()

	return madeProgress
}

func (gmmu *GMMU) removeCompletedTranslations() {
	if len(gmmu.toRemoveFromPTW) == 0 {
		return
	}

	toRemoveSet := make(map[int]bool, len(gmmu.toRemoveFromPTW))
	for _, idx := range gmmu.toRemoveFromPTW {
		toRemoveSet[idx] = true
	}

	tmp := gmmu.walkingTranslations[:0]
	for i := 0; i < len(gmmu.walkingTranslations); i++ {
		if !toRemoveSet[i] {
			tmp = append(tmp, gmmu.walkingTranslations[i])
		}
	}
	gmmu.walkingTranslations = tmp
	gmmu.toRemoveFromPTW = nil
}

func (gmmu *GMMU) processRemoteMemReq(walkingIndex int) bool {
	if !gmmu.bottomPort.CanSend() {
		return false
	}

	spec := gmmu.GetSpec()
	walking := gmmu.walkingTranslations[walkingIndex]

	req := vm.TranslationReqBuilder{}.
		WithSrc(gmmu.bottomPort.AsRemote()).
		WithDst(spec.LowModule).
		WithPID(walking.reqPayload.PID).
		WithVAddr(walking.reqPayload.VAddr).
		WithDeviceID(walking.reqPayload.DeviceID).
		Build()

	gmmu.remoteMemReqs[req.ID] = gmmu.walkingTranslations[walkingIndex]

	gmmu.bottomPort.Send(req)

	gmmu.toRemoveFromPTW = append(gmmu.toRemoveFromPTW, walkingIndex)

	return true
}

func (gmmu *GMMU) finalizePageWalk(
	walkingIndex int,
) bool {
	payload := gmmu.walkingTranslations[walkingIndex].reqPayload
	page, found := gmmu.pageTable.Find(payload.PID, payload.VAddr)
	if !found {
		return false
	}

	gmmu.walkingTranslations[walkingIndex].page = page

	return gmmu.doPageWalkHit(walkingIndex)
}

func (gmmu *GMMU) doPageWalkHit(
	walkingIndex int,
) bool {
	if !gmmu.topPort.CanSend() {
		return false
	}
	walking := gmmu.walkingTranslations[walkingIndex]

	rsp := vm.TranslationRspBuilder{}.
		WithSrc(gmmu.topPort.AsRemote()).
		WithDst(walking.req.Src).
		WithRspTo(walking.req.ID).
		WithPage(walking.page).
		Build()

	gmmu.topPort.Send(rsp)

	gmmu.toRemoveFromPTW = append(gmmu.toRemoveFromPTW, walkingIndex)

	tracing.TraceReqComplete(walking.req, gmmu)

	return true
}

func (gmmu *GMMU) fetchFromBottom() bool {
	if !gmmu.topPort.CanSend() {
		return false
	}

	rsp := gmmu.bottomPort.RetrieveIncoming()
	if rsp == nil {
		return false
	}

	tracing.TraceReqReceive(rsp, gmmu)

	switch rsp.Payload.(type) {
	case *vm.TranslationRspPayload:
		return gmmu.handleTranslationRsp(rsp)
	default:
		log.Panicf("gmmu cannot handle request of type %s", reflect.TypeOf(rsp.Payload))
		return false
	}
}

func (gmmu *GMMU) handleTranslationRsp(response *sim.Msg) bool {
	rspPayload := sim.MsgPayload[vm.TranslationRspPayload](response)
	reqTransaction := gmmu.remoteMemReqs[response.RspTo]

	if reqTransaction.req == nil {
		log.Panicf("Cannot find matching request for response %+v", response)
	}

	if !gmmu.topPort.CanSend() {
		return false
	}

	rsp := vm.TranslationRspBuilder{}.
		WithSrc(gmmu.topPort.AsRemote()).
		WithDst(reqTransaction.req.Src).
		WithRspTo(response.ID).
		WithPage(rspPayload.Page).
		Build()

	gmmu.topPort.Send(rsp)

	delete(gmmu.remoteMemReqs, response.RspTo)
	return true
}
