package mmu

import (
	"io"
	"log"
	"reflect"

	"github.com/sarchlab/akita/v5/mem/vm"
	"github.com/sarchlab/akita/v5/modeling"
	"github.com/sarchlab/akita/v5/sim"
	"github.com/sarchlab/akita/v5/tracing"
)

// Spec contains immutable configuration for the MMU.
type Spec struct {
	Latency                  int            `json:"latency"`
	MaxRequestsInFlight      int            `json:"max_requests_in_flight"`
	MigrationQueueSize       int            `json:"migration_queue_size"`
	AutoPageAllocation       bool           `json:"auto_page_allocation"`
	Log2PageSize             uint64         `json:"log2_page_size"`
	MigrationServiceProvider sim.RemotePort `json:"migration_service_provider"`
}

// pageState is a serializable representation of vm.Page.
type pageState struct {
	PID         uint32 `json:"pid"`
	VAddr       uint64 `json:"vaddr"`
	PAddr       uint64 `json:"paddr"`
	PageSize    uint64 `json:"page_size"`
	Valid       bool   `json:"valid"`
	DeviceID    uint64 `json:"device_id"`
	Unified     bool   `json:"unified"`
	IsMigrating bool   `json:"is_migrating"`
	IsPinned    bool   `json:"is_pinned"`
}

// transactionState is a serializable representation of a runtime transaction.
type transactionState struct {
	ReqID          string         `json:"req_id"`
	ReqSrc         sim.RemotePort `json:"req_src"`
	ReqDst         sim.RemotePort `json:"req_dst"`
	PID            uint32         `json:"pid"`
	VAddr          uint64         `json:"vaddr"`
	DeviceID       uint64         `json:"device_id"`
	TransLatency   uint64         `json:"trans_latency"`
	Page           pageState      `json:"page"`
	CycleLeft      int            `json:"cycle_left"`
	MigrationReqID  string         `json:"migration_req_id"`
	MigrationReqSrc sim.RemotePort `json:"migration_req_src"`
	MigrationReqDst sim.RemotePort `json:"migration_req_dst"`
	HasMigration    bool           `json:"has_migration"`
}

// devicePageAccess is a serializable replacement for map[uint64][]uint64,
// since map keys must be strings for state validation.
type devicePageAccess struct {
	PageVAddr  uint64   `json:"page_vaddr"`
	DeviceIDs  []uint64 `json:"device_ids"`
}

// State contains mutable runtime data for the MMU.
// Runtime structs with *sim.Msg remain on Comp for runtime use;
// State holds parallel serializable versions for checkpoint/restore.
type State struct {
	WalkingTranslations      []transactionState `json:"walking_translations"`
	MigrationQueue           []transactionState `json:"migration_queue"`
	CurrentOnDemandMigration transactionState   `json:"current_on_demand_migration"`
	IsDoingMigration         bool               `json:"is_doing_migration"`
	PageAccessedByDeviceID   []devicePageAccess `json:"page_accessed_by_device_id"`
	NextPhysicalPage         uint64             `json:"next_physical_page"`
	ToRemoveFromPTW          []int              `json:"to_remove_from_ptw"`
}

type transaction struct {
	req        *sim.Msg // payload: *vm.TranslationReqPayload
	reqPayload *vm.TranslationReqPayload
	page       vm.Page
	cycleLeft  int
	migration  *sim.Msg // payload: *vm.PageMigrationReqToDriverPayload
}

// Comp is the default mmu implementation. It is also an akita Component.
type Comp struct {
	*modeling.Component[Spec, State]

	topPort       sim.Port
	migrationPort sim.Port

	pageTable vm.PageTable

	walkingTranslations      []transaction
	migrationQueue           []transaction
	currentOnDemandMigration transaction
	isDoingMigration         bool

	toRemoveFromPTW        []int
	PageAccessedByDeviceID map[uint64][]uint64

	// Physical page allocation tracking for auto page allocation
	nextPhysicalPage uint64
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
	spec := m.GetSpec()
	payload := m.walkingTranslations[walkingIndex].reqPayload
	page, found := m.pageTable.Find(payload.PID, payload.VAddr)

	if !found {
		if spec.AutoPageAllocation {
			page = m.createDefaultPage(payload.PID, payload.VAddr, payload.DeviceID)
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
	spec := m.GetSpec()
	if len(m.migrationQueue) >= spec.MigrationQueueSize {
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
	if walking.reqPayload.DeviceID == walking.page.DeviceID {
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
	payload := trans.reqPayload
	page, found := m.pageTable.Find(payload.PID, payload.VAddr)

	if !found {
		panic("page not found")
	}

	trans.page = page

	if payload.DeviceID == page.DeviceID || page.IsPinned {
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
	payload := m.currentOnDemandMigration.reqPayload
	page, found := m.pageTable.Find(payload.PID, payload.VAddr)

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
	spec := m.GetSpec()
	if len(m.walkingTranslations) >= spec.MaxRequestsInFlight {
		return false
	}

	req := m.topPort.RetrieveIncoming()
	if req == nil {
		return false
	}

	tracing.TraceReqReceive(req, m.Comp)

	switch req.Payload.(type) {
	case *vm.TranslationReqPayload:
		m.startWalking(req)
	default:
		log.Panicf("MMU canot handle request of type %s", reflect.TypeOf(req.Payload))
	}

	return true
}

func (m *middleware) startWalking(req *sim.Msg) {
	spec := m.GetSpec()
	payload := sim.MsgPayload[vm.TranslationReqPayload](req)
	translationInPipeline := transaction{
		req:        req,
		reqPayload: payload,
		cycleLeft:  spec.Latency,
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
	spec := m.GetSpec()
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

func (m *middleware) allocatePhysicalPage() uint64 {
	spec := m.GetSpec()
	pageSize := uint64(1) << spec.Log2PageSize

	for {
		candidatePage := (m.nextPhysicalPage >> spec.Log2PageSize) << spec.Log2PageSize

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
) *sim.Msg {
	spec := m.GetSpec()
	migrationInfo := new(vm.PageMigrationInfo)
	migrationInfo.GPUReqToVAddrMap = make(map[uint64][]uint64)
	migrationInfo.GPUReqToVAddrMap[trans.reqPayload.DeviceID] =
		append(migrationInfo.GPUReqToVAddrMap[trans.reqPayload.DeviceID],
			trans.reqPayload.VAddr)

	m.PageAccessedByDeviceID[page.VAddr] =
		append(m.PageAccessedByDeviceID[page.VAddr], page.DeviceID)

	migrationReq := vm.NewPageMigrationReqToDriver(
		m.migrationPort.AsRemote(), spec.MigrationServiceProvider)
	migrationPayload := sim.MsgPayload[vm.PageMigrationReqToDriverPayload](migrationReq)
	migrationPayload.PID = page.PID
	migrationPayload.PageSize = page.PageSize
	migrationPayload.CurrPageHostGPU = page.DeviceID
	migrationPayload.MigrationInfo = migrationInfo
	migrationPayload.CurrAccessingGPUs = unique(
		m.PageAccessedByDeviceID[page.VAddr])
	migrationPayload.RespondToTop = true

	return migrationReq
}

// pageToState converts a vm.Page to a serializable pageState.
func pageToState(p vm.Page) pageState {
	return pageState{
		PID:         uint32(p.PID),
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

// stateToPage converts a serializable pageState back to vm.Page.
func stateToPage(ps pageState) vm.Page {
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

// transToState converts a runtime transaction to a serializable transactionState.
func transToState(t transaction) transactionState {
	ts := transactionState{
		CycleLeft: t.cycleLeft,
		Page:      pageToState(t.page),
	}

	if t.req != nil {
		ts.ReqID = t.req.ID
		ts.ReqSrc = t.req.Src
		ts.ReqDst = t.req.Dst
	}

	if t.reqPayload != nil {
		ts.PID = uint32(t.reqPayload.PID)
		ts.VAddr = t.reqPayload.VAddr
		ts.DeviceID = t.reqPayload.DeviceID
		ts.TransLatency = t.reqPayload.TransLatency
	}

	if t.migration != nil {
		ts.HasMigration = true
		ts.MigrationReqID = t.migration.ID
		ts.MigrationReqSrc = t.migration.Src
		ts.MigrationReqDst = t.migration.Dst
	}

	return ts
}

// stateToTrans converts a serializable transactionState back to a runtime
// transaction. The *sim.Msg is reconstructed with enough data to send responses.
func stateToTrans(ts transactionState) transaction {
	payload := &vm.TranslationReqPayload{
		PID:          vm.PID(ts.PID),
		VAddr:        ts.VAddr,
		DeviceID:     ts.DeviceID,
		TransLatency: ts.TransLatency,
	}

	req := &sim.Msg{
		MsgMeta: sim.MsgMeta{
			ID:  ts.ReqID,
			Src: ts.ReqSrc,
			Dst: ts.ReqDst,
		},
		Payload: payload,
	}

	t := transaction{
		req:        req,
		reqPayload: payload,
		page:       stateToPage(ts.Page),
		cycleLeft:  ts.CycleLeft,
	}

	if ts.HasMigration {
		t.migration = &sim.Msg{
			MsgMeta: sim.MsgMeta{
				ID:  ts.MigrationReqID,
				Src: ts.MigrationReqSrc,
				Dst: ts.MigrationReqDst,
			},
		}
	}

	return t
}

// GetState converts runtime mutable data into a serializable State.
func (c *Comp) GetState() State {
	state := State{
		IsDoingMigration:         c.isDoingMigration,
		CurrentOnDemandMigration: transToState(c.currentOnDemandMigration),
		NextPhysicalPage:         c.nextPhysicalPage,
	}

	if len(c.toRemoveFromPTW) > 0 {
		state.ToRemoveFromPTW = make([]int, len(c.toRemoveFromPTW))
		copy(state.ToRemoveFromPTW, c.toRemoveFromPTW)
	}

	state.WalkingTranslations = make([]transactionState, len(c.walkingTranslations))
	for i, t := range c.walkingTranslations {
		state.WalkingTranslations[i] = transToState(t)
	}

	state.MigrationQueue = make([]transactionState, len(c.migrationQueue))
	for i, t := range c.migrationQueue {
		state.MigrationQueue[i] = transToState(t)
	}

	state.PageAccessedByDeviceID = make([]devicePageAccess, 0, len(c.PageAccessedByDeviceID))
	for vaddr, deviceIDs := range c.PageAccessedByDeviceID {
		ids := make([]uint64, len(deviceIDs))
		copy(ids, deviceIDs)
		state.PageAccessedByDeviceID = append(state.PageAccessedByDeviceID, devicePageAccess{
			PageVAddr: vaddr,
			DeviceIDs: ids,
		})
	}

	c.Component.SetState(state)

	return state
}

// SetState restores runtime mutable data from a serializable State.
func (c *Comp) SetState(state State) {
	c.Component.SetState(state)

	c.isDoingMigration = state.IsDoingMigration
	c.currentOnDemandMigration = stateToTrans(state.CurrentOnDemandMigration)
	c.nextPhysicalPage = state.NextPhysicalPage

	if len(state.ToRemoveFromPTW) > 0 {
		c.toRemoveFromPTW = make([]int, len(state.ToRemoveFromPTW))
		copy(c.toRemoveFromPTW, state.ToRemoveFromPTW)
	} else {
		c.toRemoveFromPTW = nil
	}

	c.walkingTranslations = make([]transaction, len(state.WalkingTranslations))
	for i, ts := range state.WalkingTranslations {
		c.walkingTranslations[i] = stateToTrans(ts)
	}

	c.migrationQueue = make([]transaction, len(state.MigrationQueue))
	for i, ts := range state.MigrationQueue {
		c.migrationQueue[i] = stateToTrans(ts)
	}

	c.PageAccessedByDeviceID = make(map[uint64][]uint64, len(state.PageAccessedByDeviceID))
	for _, dpa := range state.PageAccessedByDeviceID {
		ids := make([]uint64, len(dpa.DeviceIDs))
		copy(ids, dpa.DeviceIDs)
		c.PageAccessedByDeviceID[dpa.PageVAddr] = ids
	}
}

// SaveState syncs runtime data to state, then delegates to Component.SaveState.
func (c *Comp) SaveState(w io.Writer) error {
	c.GetState()
	return c.Component.SaveState(w)
}

// LoadState loads state from the reader, then restores runtime fields.
func (c *Comp) LoadState(r io.Reader) error {
	if err := c.Component.LoadState(r); err != nil {
		return err
	}

	c.SetState(c.Component.GetState())

	return nil
}
