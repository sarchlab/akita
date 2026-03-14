package gmmu

import "github.com/sarchlab/akita/v5/sim"

// Spec contains immutable configuration for the GMMU.
type Spec struct {
	Freq                sim.Freq       `json:"freq"`
	DeviceID            uint64         `json:"device_id"`
	Log2PageSize        uint64         `json:"log2_page_size"`
	Latency             int            `json:"latency"`
	MaxRequestsInFlight int            `json:"max_requests_in_flight"`
	LowModule           sim.RemotePort `json:"low_module"`

	// MigrationServiceProvider is the port used for page migration requests.
	MigrationServiceProvider sim.RemotePort `json:"migration_service_provider"`
}
