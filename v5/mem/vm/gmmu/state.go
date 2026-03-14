package gmmu

import (
	"github.com/sarchlab/akita/v5/mem/vm"
	"github.com/sarchlab/akita/v5/sim"
)

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
