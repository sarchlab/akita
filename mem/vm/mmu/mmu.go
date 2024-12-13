package mmu

import (
	"log"
	"reflect"

	"github.com/sarchlab/akita/v4/mem/vm"
	"github.com/sarchlab/akita/v4/sim/hooking"
	"github.com/sarchlab/akita/v4/sim/id"
	"github.com/sarchlab/akita/v4/sim/modeling"
)

type transaction struct {
	req       vm.TranslationReq
	page      vm.Page
	cycleLeft int
	migration vm.PageMigrationReqToDriver
}

// Comp is the default mmu implementation. It is also an akita Component.
type Comp struct {
	modeling.TickingComponent
	modeling.MiddlewareHolder

	topPort       modeling.Port
	migrationPort modeling.Port

	MigrationServiceProvider modeling.RemotePort

	pageTable           vm.PageTable
	latency             int
	maxRequestsInFlight int

	walkingTranslations      []transaction
	migrationQueue           []transaction
	migrationQueueSize       int
	currentOnDemandMigration transaction
	isDoingMigration         bool

	toRemoveFromPTW        []int
	PageAccessedByDeviceID map[uint64][]uint64
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
		panic("page not found")
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
	rsp := vm.TranslationRsp{
		MsgMeta: modeling.MsgMeta{
			ID:  id.Generate(),
			Src: m.topPort.AsRemote(),
			Dst: walking.req.Src,
		},
		RespondTo: walking.req.ID,
		Page:      walking.page,
	}

	m.topPort.Send(rsp)
	m.toRemoveFromPTW = append(m.toRemoveFromPTW, walkingIndex)

	m.traceTranslationComplete(walking.req)

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

	rsp := vm.TranslationRsp{
		MsgMeta: modeling.MsgMeta{
			ID:  id.Generate(),
			Src: m.topPort.AsRemote(),
			Dst: req.Src,
		},
		RespondTo: req.ID,
		Page:      page,
	}

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

	rsp := vm.TranslationRsp{
		MsgMeta: modeling.MsgMeta{
			ID:  id.Generate(),
			Src: m.topPort.AsRemote(),
			Dst: req.Src,
		},
		RespondTo: req.ID,
		Page:      page,
	}

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

	m.traceTranslationStart(req.(vm.TranslationReq))

	switch req := req.(type) {
	case vm.TranslationReq:
		m.startWalking(req)
	default:
		log.Panicf("MMU canot handle request of type %s", reflect.TypeOf(req))
	}

	return true
}

func (m *middleware) startWalking(req vm.TranslationReq) {
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

func (m *middleware) createMigrationRequest(
	trans transaction,
	page vm.Page,
) vm.PageMigrationReqToDriver {
	migrationInfo := new(vm.PageMigrationInfo)
	migrationInfo.GPUReqToVAddrMap = make(map[uint64][]uint64)
	migrationInfo.GPUReqToVAddrMap[trans.req.DeviceID] =
		append(migrationInfo.GPUReqToVAddrMap[trans.req.DeviceID],
			trans.req.VAddr)

	m.PageAccessedByDeviceID[page.VAddr] =
		append(m.PageAccessedByDeviceID[page.VAddr], page.DeviceID)

	migrationReq := vm.PageMigrationReqToDriver{
		MsgMeta: modeling.MsgMeta{
			ID:  id.Generate(),
			Src: m.migrationPort.AsRemote(),
			Dst: m.MigrationServiceProvider,
		},
		PID:             page.PID,
		PageSize:        page.PageSize,
		CurrPageHostGPU: page.DeviceID,
		MigrationInfo:   migrationInfo,
		CurrAccessingGPUs: unique(
			m.PageAccessedByDeviceID[page.VAddr]),
		RespondToTop: true,
	}

	return migrationReq
}

func (m *middleware) traceTranslationStart(
	req vm.TranslationReq,
) {
	ctx := hooking.HookCtx{
		Domain: m.Comp,
		Item: hooking.TaskStart{
			ID:       modeling.ReqInTaskID(req.Meta().ID),
			ParentID: modeling.ReqInTaskID(req.Meta().ID),
			Kind:     "req_in",
			What:     reflect.TypeOf(req).String(),
		},
		Pos: hooking.HookPosTaskStart,
	}

	m.Comp.InvokeHook(ctx)
}

func (m *middleware) traceTranslationComplete(
	req vm.TranslationReq,
) {
	ctx := hooking.HookCtx{
		Domain: m.Comp,
		Item: hooking.TaskEnd{
			ID: modeling.ReqOutTaskID(req.Meta().ID),
		},
		Pos: hooking.HookPosTaskEnd,
	}

	m.Comp.InvokeHook(ctx)
}
