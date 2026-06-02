package gmmu

import (
	"github.com/sarchlab/akita/v5/mem/vm"
	"github.com/sarchlab/akita/v5/messaging"
	"github.com/sarchlab/akita/v5/modeling"
	"github.com/sarchlab/akita/v5/timing"
)

// Spec contains immutable configuration for the GMMU.
type Spec struct {
	Freq                timing.Freq          `json:"freq"`
	DeviceID            uint64               `json:"device_id"`
	Log2PageSize        uint64               `json:"log2_page_size"`
	Latency             int                  `json:"latency"`
	MaxRequestsInFlight int                  `json:"max_requests_in_flight"`
	LowModule           messaging.RemotePort `json:"low_module"`

	TopPortBufferSize    int `json:"top_port_buffer_size"`
	BottomPortBufferSize int `json:"bottom_port_buffer_size"`
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
	ReqID      uint64               `json:"req_id"`
	RecvTaskID uint64               `json:"recv_task_id"`
	ReqSrc     messaging.RemotePort `json:"req_src"`
	ReqDst     messaging.RemotePort `json:"req_dst"`
	PID        uint64               `json:"pid"`
	VAddr      uint64               `json:"vaddr"`
	DeviceID   uint64               `json:"device_id"`
	Page       pageState            `json:"page"`
	CycleLeft  int                  `json:"cycle_left"`
}

// devicePageAccess records pages accessed by a single device.
type devicePageAccess struct {
	DeviceID   uint64   `json:"device_id"`
	PageVAddrs []uint64 `json:"page_vaddrs"`
}

// State contains mutable runtime data for the GMMU.
type State struct {
	WalkingTranslations    []transactionState          `json:"walking_translations"`
	RemoteMemReqs          map[uint64]transactionState `json:"remote_mem_reqs"`
	ToRemoveFromPTW        []int                       `json:"to_remove_from_ptw"`
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

// Resources holds the shared resources referenced by the GMMU, such as the
// page table used for translations. These are injected through WithResources;
// they are wiring to objects shared with other components rather than scalar
// configuration.
type Resources struct {
	PageTable vm.PageTable `json:"-"`
}

// Comp is the GMMU component, a modeling.Component specialized to this
// package's Spec, State, and Resources.
type Comp = modeling.Component[Spec, State, Resources]
