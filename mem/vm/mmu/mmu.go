package mmu

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
	migration *vm.PageMigrationReqToDriver
}

// Comp is the default mmu implementation. It is also an akita Component.
type Comp struct {
	sim.TickingComponent
	sim.MiddlewareHolder

	topPort       sim.Port
	migrationPort sim.Port

	MigrationServiceProvider sim.RemotePort

	pageTable           vm.PageTable
	latency             int
	maxRequestsInFlight int
	autoPageAllocation  bool
	log2PageSize        uint64

	walkingTranslations      []transaction
	migrationQueue           []transaction
	migrationQueueSize       int
	currentOnDemandMigration transaction
	isDoingMigration         bool

	toRemoveFromPTW        []int
	PageAccessedByDeviceID map[uint64][]uint64

	// Physical page allocation tracking for auto page allocation
	nextPhysicalPage uint64
}

func (c *Comp) Tick() bool {
	return c.MiddlewareHolder.Tick()
}

type middleware struct {
	*Comp
}

// Tick defines how the MMU update state each cycle
func (m *middleware) Tick() bool {
	madeProgress := false

	madeProgress = m.sendMigrationToDriver() || madeProgress
	madeProgress = m.walkPageTable() || madeProgress
	madeProgress = m.processMigrationReturn() || madeProgress
	madeProgress = m.parseFromTop() || madeProgress

	return madeProgress
}

func (m *middleware) walkPageTable() bool {
	madeProgress := false

	for i := 0; i < len(m.walkingTranslations); i++ {
		if m.walkingTranslations[i].cycleLeft > 0 {
			m.walkingTranslations[i].cycleLeft--
			madeProgress = true

			continue
		}

		madeProgress = m.finalizePageWalk(i) || madeProgress
	}

	tmp := m.walkingTranslations[:0]

	for i := 0; i < len(m.walkingTranslations); i++ {
		if !m.toRemove(i) {
			tmp = append(tmp, m.walkingTranslations[i])
		}
	}

	m.walkingTranslations = tmp
	m.toRemoveFromPTW = nil

	return madeProgress
}

func (m *middleware) finalizePageWalk(
	walkingIndex int,
) bool {
	req := m.walkingTranslations[walkingIndex].req
	page, found := m.pageTable.Find(req.PID, req.VAddr)

	if !found {
		if m.autoPageAllocation {
			page = m.createDefaultPage(req.PID, req.VAddr, req.DeviceID)
			m.pageTable.Insert(page)
		} else {
			panic("page not found")
		}
	}

	m.walkingTranslations[walkingIndex].page = page

	if page.IsMigrating {
		return m.addTransactionToMigrationQueue(walkingIndex)
	}

	if m.pageNeedMigrate(m.walkingTranslations[walkingIndex]) {
		return m.addTransactionToMigrationQueue(walkingIndex)
	}

	return m.doPageWalkHit(walkingIndex)
}

func (m *middleware) addTransactionToMigrationQueue(walkingIndex int) bool {
	if len(m.migrationQueue) >= m.migrationQueueSize {
		return false
	}

	m.toRemoveFromPTW = append(m.toRemoveFromPTW, walkingIndex)
	m.migrationQueue = append(m.migrationQueue,
		m.walkingTranslations[walkingIndex])

	page := m.walkingTranslations[walkingIndex].page
	page.IsMigrating = true
	m.pageTable.Update(page)

	return true
}

func (m *middleware) pageNeedMigrate(walking transaction) bool {
	if walking.req.DeviceID == walking.page.DeviceID {
		return false
	}

	if !walking.page.Unified {
		return false
	}

	if walking.page.IsPinned {
		return false
	}

	return true
}

func (m *middleware) doPageWalkHit(
	walkingIndex int,
) bool {
	if !m.topPort.CanSend() {
		return false
	}

	walking := m.walkingTranslations[walkingIndex]
	rsp := vm.TranslationRspBuilder{}.
		WithSrc(m.topPort.AsRemote()).
		WithDst(walking.req.Src).
		WithRspTo(walking.req.ID).
		WithPage(walking.page).
		Build()

	m.topPort.Send(rsp)
	m.toRemoveFromPTW = append(m.toRemoveFromPTW, walkingIndex)

	tracing.TraceReqComplete(walking.req, m.Comp)

	return true
}

func (m *middleware) sendMigrationToDriver() (madeProgress bool) {
	if len(m.migrationQueue) == 0 {
		return false
	}

	trans := m.migrationQueue[0]
	req := trans.req
	page, found := m.pageTable.Find(req.PID, req.VAddr)

	if !found {
		panic("page not found")
	}

	trans.page = page

	if req.DeviceID == page.DeviceID || page.IsPinned {
		if !m.topPort.CanSend() {
			return false
		}

		m.sendTranslationRsp(trans)
		m.migrationQueue = m.migrationQueue[1:]
		m.markPageAsNotMigratingIfNotInTheMigrationQueue(page)

		return true
	}

	if m.isDoingMigration {
		return false
	}

	migrationReq := m.createMigrationRequest(trans, page)

	err := m.migrationPort.Send(migrationReq)
	if err != nil {
		return false
	}

	trans.page.IsMigrating = true
	m.pageTable.Update(trans.page)
	trans.migration = migrationReq
	m.isDoingMigration = true
	m.currentOnDemandMigration = trans
	m.migrationQueue = m.migrationQueue[1:]

	return true
}

func (m *middleware) markPageAsNotMigratingIfNotInTheMigrationQueue(
	page vm.Page,
) vm.Page {
	inQueue := false

	for _, t := range m.migrationQueue {
		if page.PAddr == t.page.PAddr {
			inQueue = true
			break
		}
	}

	if !inQueue {
		page.IsMigrating = false
		m.pageTable.Update(page)

		return page
	}

	return page
}

func (m *middleware) sendTranslationRsp(
	trans transaction,
) (madeProgress bool) {
	req := trans.req
	page := trans.page

	rsp := vm.TranslationRspBuilder{}.
		WithSrc(m.topPort.AsRemote()).
		WithDst(req.Src).
		WithRspTo(req.ID).
		WithPage(page).
		Build()
	m.topPort.Send(rsp)

	return true
}

func (m *middleware) processMigrationReturn() bool {
	item := m.migrationPort.PeekIncoming()
	if item == nil {
		return false
	}

	if !m.topPort.CanSend() {
		return false
	}

	req := m.currentOnDemandMigration.req
	page, found := m.pageTable.Find(req.PID, req.VAddr)

	if !found {
		panic("page not found")
	}

	rsp := vm.TranslationRspBuilder{}.
		WithSrc(m.topPort.AsRemote()).
		WithDst(req.Src).
		WithRspTo(req.ID).
		WithPage(page).
		Build()
	m.topPort.Send(rsp)

	m.isDoingMigration = false

	page = m.markPageAsNotMigratingIfNotInTheMigrationQueue(page)
	page.IsPinned = true
	m.pageTable.Update(page)

	m.migrationPort.RetrieveIncoming()

	return true
}

func (m *middleware) parseFromTop() bool {
	if len(m.walkingTranslations) >= m.maxRequestsInFlight {
		return false
	}

	req := m.topPort.RetrieveIncoming()
	if req == nil {
		return false
	}

	tracing.TraceReqReceive(req, m.Comp)

	switch req := req.(type) {
	case *vm.TranslationReq:
		m.startWalking(req)
	default:
		log.Panicf("MMU canot handle request of type %s", reflect.TypeOf(req))
	}

	return true
}

func (m *middleware) startWalking(req *vm.TranslationReq) {
	translationInPipeline := transaction{
		req:       req,
		cycleLeft: m.latency,
	}

	m.walkingTranslations = append(m.walkingTranslations, translationInPipeline)
}

func (m *middleware) toRemove(index int) bool {
	for i := 0; i < len(m.toRemoveFromPTW); i++ {
		remove := m.toRemoveFromPTW[i]
		if remove == index {
			return true
		}
	}

	return false
}

func unique(intSlice []uint64) []uint64 {
	keys := make(map[int]bool)
	list := []uint64{}

	for _, entry := range intSlice {
		if _, value := keys[int(entry)]; !value {
			keys[int(entry)] = true

			list = append(list, entry)
		}
	}

	return list
}

func (m *middleware) createDefaultPage(pid vm.PID, vAddr uint64, deviceID uint64) vm.Page {
	alignedVAddr := (vAddr >> m.log2PageSize) << m.log2PageSize
	pageSize := uint64(1) << m.log2PageSize
	pAddr := m.allocatePhysicalPage()
	
	return vm.Page{
		PID:         pid,
		VAddr:       alignedVAddr,
		PAddr:       pAddr,
		PageSize:    pageSize,
		Valid:       true,
		DeviceID:    deviceID,
		Unified:     true,
		IsMigrating: false,
		IsPinned:    false,
	}
}

func (m *middleware) allocatePhysicalPage() uint64 {
	pageSize := uint64(1) << m.log2PageSize
	
	for {
		candidatePage := (m.nextPhysicalPage >> m.log2PageSize) << m.log2PageSize
		
		if _, found := m.pageTable.ReverseLookup(candidatePage); !found {
			m.nextPhysicalPage = candidatePage + pageSize
			return candidatePage
		}
		
		m.nextPhysicalPage += pageSize
	}
}

func (m *middleware) createMigrationRequest(
	trans transaction,
	page vm.Page,
) *vm.PageMigrationReqToDriver {
	migrationInfo := new(vm.PageMigrationInfo)
	migrationInfo.GPUReqToVAddrMap = make(map[uint64][]uint64)
	migrationInfo.GPUReqToVAddrMap[trans.req.DeviceID] =
		append(migrationInfo.GPUReqToVAddrMap[trans.req.DeviceID],
			trans.req.VAddr)

	m.PageAccessedByDeviceID[page.VAddr] =
		append(m.PageAccessedByDeviceID[page.VAddr], page.DeviceID)

	migrationReq := vm.NewPageMigrationReqToDriver(
		m.migrationPort.AsRemote(), m.MigrationServiceProvider)
	migrationReq.PID = page.PID
	migrationReq.PageSize = page.PageSize
	migrationReq.CurrPageHostGPU = page.DeviceID
	migrationReq.MigrationInfo = migrationInfo
	migrationReq.CurrAccessingGPUs = unique(
		m.PageAccessedByDeviceID[page.VAddr])
	migrationReq.RespondToTop = true

	return migrationReq
}
