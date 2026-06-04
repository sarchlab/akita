package mmu

import (
	"github.com/sarchlab/akita/v5/mem/control"
	"github.com/sarchlab/akita/v5/mem/vm"
	"github.com/sarchlab/akita/v5/messaging"
	"github.com/sarchlab/akita/v5/modeling"
	"github.com/sarchlab/akita/v5/timing"
)

// Spec contains immutable configuration for the MMU.
type Spec struct {
	Freq                     timing.Freq          `json:"freq"`
	Latency                  int                  `json:"latency"`
	MaxRequestsInFlight      int                  `json:"max_requests_in_flight"`
	MigrationQueueSize       int                  `json:"migration_queue_size"`
	AutoPageAllocation       bool                 `json:"auto_page_allocation"`
	Log2PageSize             uint64               `json:"log2_page_size"`
	MigrationServiceProvider messaging.RemotePort `json:"migration_service_provider"`

	TopPortBufferSize       int `json:"top_port_buffer_size"`
	MigrationPortBufferSize int `json:"migration_port_buffer_size"`
	CtrlPortBufferSize      int `json:"ctrl_port_buffer_size"`
}

// transactionState is the canonical transaction representation.
type transactionState struct {
	ReqID        uint64               `json:"req_id"`
	RecvTaskID   uint64               `json:"recv_task_id"`
	ReqSrc       messaging.RemotePort `json:"req_src"`
	ReqDst       messaging.RemotePort `json:"req_dst"`
	PID          uint32               `json:"pid"`
	VAddr        uint64               `json:"vaddr"`
	DeviceID     uint64               `json:"device_id"`
	TransLatency uint64               `json:"trans_latency"`
	Page         vm.Page              `json:"page"`
	CycleLeft    int                  `json:"cycle_left"`

	MigrationReqID  uint64               `json:"migration_req_id"`
	MigrationReqSrc messaging.RemotePort `json:"migration_req_src"`
	MigrationReqDst messaging.RemotePort `json:"migration_req_dst"`
	HasMigration    bool                 `json:"has_migration"`
}

// devicePageAccess is a serializable replacement for map[uint64][]uint64,
// since map keys must be strings for state validation.
type devicePageAccess struct {
	PageVAddr uint64   `json:"page_vaddr"`
	DeviceIDs []uint64 `json:"device_ids"`
}

// State contains mutable runtime data for the MMU.
type State struct {
	ControlState             control.State        `json:"control_state"`
	CurrentCmdID             uint64               `json:"current_cmd_id"`
	CurrentCmdSrc            messaging.RemotePort `json:"current_cmd_src"`
	WalkingTranslations      []transactionState   `json:"walking_translations"`
	MigrationQueue           []transactionState   `json:"migration_queue"`
	CurrentOnDemandMigration transactionState     `json:"current_on_demand_migration"`
	IsDoingMigration         bool                 `json:"is_doing_migration"`
	PageAccessedByDeviceID   []devicePageAccess   `json:"page_accessed_by_device_id"`
	NextPhysicalPage         uint64               `json:"next_physical_page"`
	ToRemoveFromPTW          []int                `json:"to_remove_from_ptw"`
}

// Resources holds the shared resources referenced by the MMU. The page table is
// external wiring shared with other components.
type Resources struct {
	PageTable vm.PageTable `json:"-"`
}

// Comp is the MMU component.
type Comp = modeling.Component[Spec, State, Resources]
