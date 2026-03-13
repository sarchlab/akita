package mmu

import (
	"fmt"
	"log"

	"github.com/sarchlab/akita/v5/mem/vm"
	"github.com/sarchlab/akita/v5/modeling"
	"github.com/sarchlab/akita/v5/sim"
	"github.com/sarchlab/akita/v5/tracing"
)

// Spec contains immutable configuration for the MMU.
type Spec struct {
	Freq                     sim.Freq       `json:"freq"`
	Latency                  int            `json:"latency"`
	MaxRequestsInFlight      int            `json:"max_requests_in_flight"`
	MigrationQueueSize       int            `json:"migration_queue_size"`
	AutoPageAllocation       bool           `json:"auto_page_allocation"`
	Log2PageSize             uint64         `json:"log2_page_size"`
	MigrationServiceProvider sim.RemotePort `json:"migration_service_provider"`
}

// transactionState is the canonical transaction representation.
type transactionState struct {
	ReqID        string         `json:"req_id"`
	ReqSrc       sim.RemotePort `json:"req_src"`
	ReqDst       sim.RemotePort `json:"req_dst"`
	PID          uint32         `json:"pid"`
	VAddr        uint64         `json:"vaddr"`
	DeviceID     uint64         `json:"device_id"`
	TransLatency uint64         `json:"trans_latency"`
	Page         vm.Page        `json:"page"`
	CycleLeft    int            `json:"cycle_left"`

	MigrationReqID  string         `json:"migration_req_id"`
	MigrationReqSrc sim.RemotePort `json:"migration_req_src"`
	MigrationReqDst sim.RemotePort `json:"migration_req_dst"`
	HasMigration    bool           `json:"has_migration"`
}

// devicePageAccess is a serializable replacement for map[uint64][]uint64,
// since map keys must be strings for state validation.
type devicePageAccess struct {
	PageVAddr uint64   `json:"page_vaddr"`
	DeviceIDs []uint64 `json:"device_ids"`
}

// State contains mutable runtime data for the MMU.
type State struct {
	WalkingTranslations      []transactionState `json:"walking_translations"`
	MigrationQueue           []transactionState `json:"migration_queue"`
	CurrentOnDemandMigration transactionState   `json:"current_on_demand_migration"`
	IsDoingMigration         bool               `json:"is_doing_migration"`
	PageAccessedByDeviceID   []devicePageAccess `json:"page_accessed_by_device_id"`
	NextPhysicalPage         uint64             `json:"next_physical_page"`
	ToRemoveFromPTW          []int              `json:"to_remove_from_ptw"`
}

// PageTable returns the page table from an MMU component.
func PageTable(c *modeling.Component[Spec, State]) vm.PageTable {
	return c.Middlewares()[0].(*translationMW).pageTable
}

// translationMW handles translation requests: parsing from top,
// page table walks, and sending responses for local hits.
type translationMW struct {
	comp      *modeling.Component[Spec, State]
	pageTable vm.PageTable
}

// Port helpers.

func (m *translationMW) topPort() sim.Port {
	return m.comp.GetPortByName("Top")
}

// Tick runs the translation stages.
func (m *translationMW) Tick() bool {
	madeProgress := false

	madeProgress = m.walkPageTable() || madeProgress
	madeProgress = m.parseFromTop() || madeProgress

	return madeProgress
}

func (m *translationMW) walkPageTable() bool {
	madeProgress := false

	cur := m.comp.GetState()
	next := m.comp.GetNextState()

	for i := 0; i < len(cur.WalkingTranslations); i++ {
		if cur.WalkingTranslations[i].CycleLeft > 0 {
			next.WalkingTranslations[i].CycleLeft = cur.WalkingTranslations[i].CycleLeft - 1
			madeProgress = true

			continue
		}

		madeProgress = m.finalizePageWalk(i) || madeProgress
	}

	next = m.comp.GetNextState()
	tmp := next.WalkingTranslations[:0]

	for i := 0; i < len(next.WalkingTranslations); i++ {
		if !m.toRemove(i) {
			tmp = append(tmp, next.WalkingTranslations[i])
		}
	}

	next.WalkingTranslations = tmp
	next.ToRemoveFromPTW = nil

	return madeProgress
}

func (m *translationMW) finalizePageWalk(walkingIndex int) bool {
	spec := m.comp.GetSpec()
	cur := m.comp.GetState()
	next := m.comp.GetNextState()
	walking := cur.WalkingTranslations[walkingIndex]

	page, found := m.pageTable.Find(
		vm.PID(walking.PID), walking.VAddr)

	if !found {
		if spec.AutoPageAllocation {
			page = m.createDefaultPage(
				vm.PID(walking.PID), walking.VAddr, walking.DeviceID)
			m.pageTable.Insert(page)
		} else {
			panic("page not found")
		}
	}

	next.WalkingTranslations[walkingIndex].Page = page

	if page.IsMigrating {
		return m.addTransactionToMigrationQueue(walkingIndex)
	}

	if m.pageNeedMigrate(next.WalkingTranslations[walkingIndex]) {
		return m.addTransactionToMigrationQueue(walkingIndex)
	}

	return m.doPageWalkHit(walkingIndex)
}

func (m *translationMW) addTransactionToMigrationQueue(
	walkingIndex int,
) bool {
	spec := m.comp.GetSpec()
	cur := m.comp.GetState()
	next := m.comp.GetNextState()

	if len(cur.MigrationQueue) >= spec.MigrationQueueSize {
		return false
	}

	next.ToRemoveFromPTW = append(next.ToRemoveFromPTW, walkingIndex)
	next.MigrationQueue = append(next.MigrationQueue,
		next.WalkingTranslations[walkingIndex])

	page := next.WalkingTranslations[walkingIndex].Page
	page.IsMigrating = true
	m.pageTable.Update(page)

	return true
}

func (m *translationMW) pageNeedMigrate(
	walking transactionState,
) bool {
	if walking.DeviceID == walking.Page.DeviceID {
		return false
	}

	if !walking.Page.Unified {
		return false
	}

	if walking.Page.IsPinned {
		return false
	}

	return true
}

func (m *translationMW) doPageWalkHit(walkingIndex int) bool {
	if !m.topPort().CanSend() {
		return false
	}

	next := m.comp.GetNextState()
	walking := next.WalkingTranslations[walkingIndex]

	rsp := &vm.TranslationRsp{
		Page: walking.Page,
	}
	rsp.ID = sim.GetIDGenerator().Generate()
	rsp.Src = m.topPort().AsRemote()
	rsp.Dst = walking.ReqSrc
	rsp.RspTo = walking.ReqID
	rsp.TrafficClass = "vm.TranslationRsp"

	m.topPort().Send(rsp)
	next.ToRemoveFromPTW = append(next.ToRemoveFromPTW, walkingIndex)

	m.traceReqComplete(walking.ReqID)

	return true
}

func (m *translationMW) parseFromTop() bool {
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
		log.Panicf("MMU canot handle request of type %s",
			fmt.Sprintf("%T", reqI))
	}

	return true
}

func (m *translationMW) startWalking(req *vm.TranslationReq) {
	spec := m.comp.GetSpec()
	next := m.comp.GetNextState()

	ts := transactionState{
		ReqID:        req.ID,
		ReqSrc:       req.Src,
		ReqDst:       req.Dst,
		PID:          uint32(req.PID),
		VAddr:        req.VAddr,
		DeviceID:     req.DeviceID,
		TransLatency: req.TransLatency,
		CycleLeft:    spec.Latency,
	}

	next.WalkingTranslations = append(next.WalkingTranslations, ts)
}

func (m *translationMW) toRemove(index int) bool {
	next := m.comp.GetNextState()

	for i := 0; i < len(next.ToRemoveFromPTW); i++ {
		remove := next.ToRemoveFromPTW[i]
		if remove == index {
			return true
		}
	}

	return false
}

func (m *translationMW) createDefaultPage(
	pid vm.PID, vAddr uint64, deviceID uint64,
) vm.Page {
	spec := m.comp.GetSpec()
	alignedVAddr := (vAddr >> spec.Log2PageSize) << spec.Log2PageSize
	pageSize := uint64(1) << spec.Log2PageSize
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

func (m *translationMW) allocatePhysicalPage() uint64 {
	spec := m.comp.GetSpec()
	next := m.comp.GetNextState()
	pageSize := uint64(1) << spec.Log2PageSize

	for {
		candidatePage := (next.NextPhysicalPage >> spec.Log2PageSize) << spec.Log2PageSize

		if _, found := m.pageTable.ReverseLookup(candidatePage); !found {
			next.NextPhysicalPage = candidatePage + pageSize
			return candidatePage
		}

		next.NextPhysicalPage += pageSize
	}
}

func (m *translationMW) traceReqComplete(reqID string) {
	taskID := fmt.Sprintf("%s@%s", reqID, m.comp.Name())
	tracing.EndTask(taskID, m.comp)
}

// migrationMW handles migration: sending migration requests to the driver,
// processing migration returns, and sending translation responses.
type migrationMW struct {
	comp      *modeling.Component[Spec, State]
	pageTable vm.PageTable
}

// Port helpers.

func (m *migrationMW) topPort() sim.Port {
	return m.comp.GetPortByName("Top")
}

func (m *migrationMW) migrationPort() sim.Port {
	return m.comp.GetPortByName("Migration")
}

// Tick runs the migration stages.
func (m *migrationMW) Tick() bool {
	madeProgress := false

	madeProgress = m.sendMigrationToDriver() || madeProgress
	madeProgress = m.processMigrationReturn() || madeProgress

	return madeProgress
}

func (m *migrationMW) sendMigrationToDriver() (madeProgress bool) {
	cur := m.comp.GetState()

	if len(cur.MigrationQueue) == 0 {
		return false
	}

	next := m.comp.GetNextState()

	trans := cur.MigrationQueue[0]
	page, found := m.pageTable.Find(
		vm.PID(trans.PID), trans.VAddr)

	if !found {
		panic("page not found")
	}

	trans.Page = page

	if trans.DeviceID == page.DeviceID || page.IsPinned {
		if !m.topPort().CanSend() {
			return false
		}

		m.sendTranslationRsp(trans)
		next.MigrationQueue = next.MigrationQueue[1:]
		m.markPageAsNotMigratingIfNotInTheMigrationQueue(page)

		return true
	}

	if cur.IsDoingMigration {
		return false
	}

	migrationReq := m.createMigrationRequest(trans, page)

	err := m.migrationPort().Send(migrationReq)
	if err != nil {
		return false
	}

	trans.Page.IsMigrating = true
	m.pageTable.Update(trans.Page)
	trans.HasMigration = true
	trans.MigrationReqID = migrationReq.ID
	trans.MigrationReqSrc = migrationReq.Src
	trans.MigrationReqDst = migrationReq.Dst
	next.IsDoingMigration = true
	next.CurrentOnDemandMigration = trans
	next.MigrationQueue = next.MigrationQueue[1:]

	return true
}

func (m *migrationMW) markPageAsNotMigratingIfNotInTheMigrationQueue(
	page vm.Page,
) vm.Page {
	next := m.comp.GetNextState()
	inQueue := false

	for _, t := range next.MigrationQueue {
		if page.PAddr == t.Page.PAddr {
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

func (m *migrationMW) sendTranslationRsp(
	trans transactionState,
) (madeProgress bool) {
	rsp := &vm.TranslationRsp{
		Page: trans.Page,
	}
	rsp.ID = sim.GetIDGenerator().Generate()
	rsp.Src = m.topPort().AsRemote()
	rsp.Dst = trans.ReqSrc
	rsp.RspTo = trans.ReqID
	rsp.TrafficClass = "vm.TranslationRsp"
	m.topPort().Send(rsp)

	return true
}

func (m *migrationMW) processMigrationReturn() bool {
	item := m.migrationPort().PeekIncoming()
	if item == nil {
		return false
	}

	if !m.topPort().CanSend() {
		return false
	}

	cur := m.comp.GetState()
	next := m.comp.GetNextState()
	trans := cur.CurrentOnDemandMigration

	page, found := m.pageTable.Find(
		vm.PID(trans.PID), trans.VAddr)

	if !found {
		panic("page not found")
	}

	rsp := &vm.TranslationRsp{
		Page: page,
	}
	rsp.ID = sim.GetIDGenerator().Generate()
	rsp.Src = m.topPort().AsRemote()
	rsp.Dst = trans.ReqSrc
	rsp.RspTo = trans.ReqID
	rsp.TrafficClass = "vm.TranslationRsp"
	m.topPort().Send(rsp)

	next.IsDoingMigration = false

	page = m.markPageAsNotMigratingIfNotInTheMigrationQueue(page)
	page.IsPinned = true
	m.pageTable.Update(page)

	m.migrationPort().RetrieveIncoming()

	return true
}

func (m *migrationMW) createMigrationRequest(
	trans transactionState,
	page vm.Page,
) *vm.PageMigrationReqToDriver {
	spec := m.comp.GetSpec()
	next := m.comp.GetNextState()

	migrationInfo := new(vm.PageMigrationInfo)
	migrationInfo.GPUReqToVAddrMap = make(map[uint64][]uint64)
	migrationInfo.GPUReqToVAddrMap[trans.DeviceID] =
		append(migrationInfo.GPUReqToVAddrMap[trans.DeviceID],
			trans.VAddr)

	next.PageAccessedByDeviceID = appendDeviceID(
		next.PageAccessedByDeviceID, page.VAddr, page.DeviceID)

	migrationReq := vm.NewPageMigrationReqToDriver(
		m.migrationPort().AsRemote(), spec.MigrationServiceProvider)
	migrationReq.PID = page.PID
	migrationReq.PageSize = page.PageSize
	migrationReq.CurrPageHostGPU = page.DeviceID
	migrationReq.MigrationInfo = migrationInfo
	migrationReq.CurrAccessingGPUs = unique(
		getDeviceIDs(next.PageAccessedByDeviceID, page.VAddr))
	migrationReq.RespondToTop = true

	return migrationReq
}

// Helper functions for devicePageAccess slice.

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

func getDeviceIDs(accesses []devicePageAccess, vaddr uint64) []uint64 {
	for _, a := range accesses {
		if a.PageVAddr == vaddr {
			return a.DeviceIDs
		}
	}

	return nil
}

func appendDeviceID(
	accesses []devicePageAccess, vaddr, deviceID uint64,
) []devicePageAccess {
	for i, a := range accesses {
		if a.PageVAddr == vaddr {
			accesses[i].DeviceIDs = append(a.DeviceIDs, deviceID)
			return accesses
		}
	}

	return append(accesses, devicePageAccess{
		PageVAddr: vaddr,
		DeviceIDs: []uint64{deviceID},
	})
}
