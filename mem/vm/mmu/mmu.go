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

	topPort       sim.Port
	migrationPort sim.Port

	MigrationServiceProvider sim.Port

	topSender sim.BufferedSender

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

// Tick defines how the MMU update state each cycle
func (c *Comp) Tick() bool {
	madeProgress := false

	madeProgress = c.topSender.Tick() || madeProgress
	madeProgress = c.sendMigrationToDriver() || madeProgress
	madeProgress = c.walkPageTable() || madeProgress
	madeProgress = c.processMigrationReturn() || madeProgress
	madeProgress = c.parseFromTop() || madeProgress

	return madeProgress
}

func (c *Comp) walkPageTable() bool {
	madeProgress := false
	for i := 0; i < len(c.walkingTranslations); i++ {
		if c.walkingTranslations[i].cycleLeft > 0 {
			c.walkingTranslations[i].cycleLeft--
			madeProgress = true
			continue
		}

		madeProgress = c.finalizePageWalk(i) || madeProgress
	}

	tmp := c.walkingTranslations[:0]
	for i := 0; i < len(c.walkingTranslations); i++ {
		if !c.toRemove(i) {
			tmp = append(tmp, c.walkingTranslations[i])
		}
	}
	c.walkingTranslations = tmp
	c.toRemoveFromPTW = nil

	return madeProgress
}

func (c *Comp) finalizePageWalk(
	walkingIndex int,
) bool {
	req := c.walkingTranslations[walkingIndex].req
	page, found := c.pageTable.Find(req.PID, req.VAddr)

	if !found {
		panic("page not found")
	}

	c.walkingTranslations[walkingIndex].page = page

	if page.IsMigrating {
		return c.addTransactionToMigrationQueue(walkingIndex)
	}

	if c.pageNeedMigrate(c.walkingTranslations[walkingIndex]) {
		return c.addTransactionToMigrationQueue(walkingIndex)
	}

	return c.doPageWalkHit(walkingIndex)
}

func (c *Comp) addTransactionToMigrationQueue(walkingIndex int) bool {
	if len(c.migrationQueue) >= c.migrationQueueSize {
		return false
	}

	c.toRemoveFromPTW = append(c.toRemoveFromPTW, walkingIndex)
	c.migrationQueue = append(c.migrationQueue,
		c.walkingTranslations[walkingIndex])

	page := c.walkingTranslations[walkingIndex].page
	page.IsMigrating = true
	c.pageTable.Update(page)

	return true
}

func (c *Comp) pageNeedMigrate(walking transaction) bool {
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

func (c *Comp) doPageWalkHit(
	walkingIndex int,
) bool {
	if !c.topSender.CanSend(1) {
		return false
	}
	walking := c.walkingTranslations[walkingIndex]

	rsp := vm.TranslationRspBuilder{}.
		WithSrc(c.topPort).
		WithDst(walking.req.Src).
		WithRspTo(walking.req.ID).
		WithPage(walking.page).
		Build()

	c.topSender.Send(rsp)
	c.toRemoveFromPTW = append(c.toRemoveFromPTW, walkingIndex)

	tracing.TraceReqComplete(walking.req, c)

	return true
}

func (c *Comp) sendMigrationToDriver() (madeProgress bool) {
	if len(c.migrationQueue) == 0 {
		return false
	}

	trans := c.migrationQueue[0]
	req := trans.req
	page, found := c.pageTable.Find(req.PID, req.VAddr)
	if !found {
		panic("page not found")
	}
	trans.page = page

	if req.DeviceID == page.DeviceID || page.IsPinned {
		c.sendTranlationRsp(trans)
		c.migrationQueue = c.migrationQueue[1:]
		c.markPageAsNotMigratingIfNotInTheMigrationQueue(page)

		return true
	}

	if c.isDoingMigration {
		return false
	}

	migrationInfo := new(vm.PageMigrationInfo)
	migrationInfo.GPUReqToVAddrMap = make(map[uint64][]uint64)
	migrationInfo.GPUReqToVAddrMap[trans.req.DeviceID] =
		append(migrationInfo.GPUReqToVAddrMap[trans.req.DeviceID],
			trans.req.VAddr)

	c.PageAccessedByDeviceID[page.VAddr] =
		append(c.PageAccessedByDeviceID[page.VAddr], page.DeviceID)

	migrationReq := vm.NewPageMigrationReqToDriver(
		c.migrationPort, c.MigrationServiceProvider)
	migrationReq.PID = page.PID
	migrationReq.PageSize = page.PageSize
	migrationReq.CurrPageHostGPU = page.DeviceID
	migrationReq.MigrationInfo = migrationInfo
	migrationReq.CurrAccessingGPUs = unique(c.PageAccessedByDeviceID[page.VAddr])
	migrationReq.RespondToTop = true

	err := c.migrationPort.Send(migrationReq)
	if err != nil {
		return false
	}

	trans.page.IsMigrating = true
	c.pageTable.Update(trans.page)
	trans.migration = migrationReq
	c.isDoingMigration = true
	c.currentOnDemandMigration = trans
	c.migrationQueue = c.migrationQueue[1:]

	return true
}

func (c *Comp) markPageAsNotMigratingIfNotInTheMigrationQueue(
	page vm.Page,
) vm.Page {
	inQueue := false
	for _, t := range c.migrationQueue {
		if page.PAddr == t.page.PAddr {
			inQueue = true
			break
		}
	}

	if !inQueue {
		page.IsMigrating = false
		c.pageTable.Update(page)
		return page
	}

	return page
}

func (c *Comp) sendTranlationRsp(
	trans transaction,
) (madeProgress bool) {
	req := trans.req
	page := trans.page

	rsp := vm.TranslationRspBuilder{}.
		WithSrc(c.topPort).
		WithDst(req.Src).
		WithRspTo(req.ID).
		WithPage(page).
		Build()
	c.topSender.Send(rsp)

	return true
}

func (c *Comp) processMigrationReturn() bool {
	item := c.migrationPort.PeekIncoming()
	if item == nil {
		return false
	}

	if !c.topSender.CanSend(1) {
		return false
	}

	req := c.currentOnDemandMigration.req
	page, found := c.pageTable.Find(req.PID, req.VAddr)
	if !found {
		panic("page not found")
	}

	rsp := vm.TranslationRspBuilder{}.
		WithSrc(c.topPort).
		WithDst(req.Src).
		WithRspTo(req.ID).
		WithPage(page).
		Build()
	c.topSender.Send(rsp)

	c.isDoingMigration = false

	page = c.markPageAsNotMigratingIfNotInTheMigrationQueue(page)
	page.IsPinned = true
	c.pageTable.Update(page)

	c.migrationPort.RetrieveIncoming()

	return true
}

func (c *Comp) parseFromTop() bool {
	if len(c.walkingTranslations) >= c.maxRequestsInFlight {
		return false
	}

	req := c.topPort.RetrieveIncoming()
	if req == nil {
		return false
	}

	tracing.TraceReqReceive(req, c)

	switch req := req.(type) {
	case *vm.TranslationReq:
		c.startWalking(req)
	default:
		log.Panicf("MMU canot handle request of type %s", reflect.TypeOf(req))
	}

	return true
}

func (c *Comp) startWalking(req *vm.TranslationReq) {
	translationInPipeline := transaction{
		req:       req,
		cycleLeft: c.latency,
	}

	c.walkingTranslations = append(c.walkingTranslations, translationInPipeline)
}

func (c *Comp) toRemove(index int) bool {
	for i := 0; i < len(c.toRemoveFromPTW); i++ {
		remove := c.toRemoveFromPTW[i]
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
