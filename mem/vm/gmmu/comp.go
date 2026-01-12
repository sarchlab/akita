package gmmu

import (
	"log"
	"reflect"

	"github.com/sarchlab/akita/v4/mem/vm"
	"github.com/sarchlab/akita/v4/sim"
	"github.com/sarchlab/akita/v4/tracing"
)

type transaction struct {
	req       *vm.TranslationReq
	page      vm.Page
	cycleLeft int
}

// gmmu is the default gmmu implementation. It is also an akita Component.
type GMMU struct {
	sim.TickingComponent

	deviceID uint64

	topPort    sim.Port
	bottomPort sim.Port

	// LowModule is the port used to communicate with the lower-level memory module.
	LowModule sim.Port

	log2PageSize uint64

	// MigrationServiceProvider is the port used for page migration requests and
	// responses between the GMMU and the migration service.
	MigrationServiceProvider sim.Port

	pageTable           vm.PageTable
	latency             int
	maxRequestsInFlight int

	walkingTranslations []transaction
	remoteMemReqs       map[uint64]transaction

	toRemoveFromPTW []int

	// PageAccessedByDeviceID records, for each device ID, the pages that have
	// been accessed.
	PageAccessedByDeviceID map[uint64][]uint64
}

// Tick defines how the gmmu update state each cycle
func (gmmu *GMMU) Tick() bool {
	madeProgress := false

	madeProgress = gmmu.walkPageTable() || madeProgress
	madeProgress = gmmu.parseFromTop() || madeProgress
	madeProgress = gmmu.fetchFromBottom() || madeProgress

	return madeProgress
}

func (gmmu *GMMU) parseFromTop() bool {
	if len(gmmu.walkingTranslations) >= gmmu.maxRequestsInFlight {
		return false
	}

	req := gmmu.topPort.RetrieveIncoming()
	if req == nil {
		return false
	}

	tracing.TraceReqReceive(req, gmmu)

	switch req := req.(type) {
	case *vm.TranslationReq:
		gmmu.startWalking(req)
	default:
		log.Panicf("gmmu cannot handle request of type %s", reflect.TypeOf(req))
	}

	return true
}

func (gmmu *GMMU) startWalking(req *vm.TranslationReq) {
	translationInPipeline := transaction{
		req:       req,
		cycleLeft: gmmu.latency,
	}

	gmmu.walkingTranslations = append(gmmu.walkingTranslations, translationInPipeline)
}

func (gmmu *GMMU) walkPageTable() bool {
	madeProgress := false

	if len(gmmu.walkingTranslations) == 0 {
		return false
	}

	for i := 0; i < len(gmmu.walkingTranslations); i++ {
		if gmmu.walkingTranslations[i].cycleLeft > 0 {
			gmmu.walkingTranslations[i].cycleLeft--
			madeProgress = true
			continue
		}
		req := gmmu.walkingTranslations[i].req

		page, found := gmmu.pageTable.Find(req.PID, req.VAddr)
		if !found {
			log.Panicf(
				"gmmu: page not found for PID %d VAddr 0x%x",
				req.PID, req.VAddr,
			)
		}

		if page.DeviceID == gmmu.deviceID {
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

	walking := gmmu.walkingTranslations[walkingIndex].req

	gmmu.remoteMemReqs[walking.VAddr] = gmmu.walkingTranslations[walkingIndex]

	req := vm.TranslationReqBuilder{}.
		WithSrc(gmmu.bottomPort.AsRemote()).
		WithDst(gmmu.LowModule.AsRemote()).
		WithPID(walking.PID).
		WithVAddr(walking.VAddr).
		WithDeviceID(walking.DeviceID).
		Build()

	gmmu.bottomPort.Send(req)

	gmmu.toRemoveFromPTW = append(gmmu.toRemoveFromPTW, walkingIndex)

	return true
}

func (gmmu *GMMU) finalizePageWalk(
	walkingIndex int,
) bool {
	req := gmmu.walkingTranslations[walkingIndex].req
	page, found := gmmu.pageTable.Find(req.PID, req.VAddr)
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

	switch rsp := rsp.(type) {
	case *vm.TranslationRsp:
		return gmmu.handleTranslationRsp(rsp)
	default:
		log.Panicf("gmmu cannot handle request of type %s", reflect.TypeOf(rsp))
		return false
	}
}

func (gmmu *GMMU) handleTranslationRsp(response *vm.TranslationRsp) bool {
	reqTransaction := gmmu.remoteMemReqs[response.Page.VAddr]

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
		WithPage(response.Page).
		Build()

	gmmu.topPort.Send(rsp)

	delete(gmmu.remoteMemReqs, response.Page.VAddr)
	return true
}
