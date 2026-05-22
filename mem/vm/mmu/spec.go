package mmu

import (
	"github.com/sarchlab/akita/v5/messaging"
	"github.com/sarchlab/akita/v5/timing"
	// Spec contains immutable configuration for the MMU.
)

type Spec struct {
	Freq                     timing.Freq          `json:"freq"`
	Latency                  int                  `json:"latency"`
	MaxRequestsInFlight      int                  `json:"max_requests_in_flight"`
	MigrationQueueSize       int                  `json:"migration_queue_size"`
	AutoPageAllocation       bool                 `json:"auto_page_allocation"`
	Log2PageSize             uint64               `json:"log2_page_size"`
	MigrationServiceProvider messaging.RemotePort `json:"migration_service_provider"`
}
