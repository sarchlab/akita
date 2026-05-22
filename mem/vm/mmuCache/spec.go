package mmuCache

import (
	"github.com/sarchlab/akita/v5/messaging"
	"github.com/sarchlab/akita/v5/timing"
)

// Spec contains immutable configuration for the mmuCache.
type Spec struct {
	Freq            timing.Freq          `json:"freq"`
	NumBlocks       int                  `json:"num_blocks"`
	NumLevels       int                  `json:"num_levels"`
	PageSize        uint64               `json:"page_size"`
	Log2PageSize    uint64               `json:"log2_page_size"`
	NumReqPerCycle  int                  `json:"num_req_per_cycle"`
	LatencyPerLevel uint64               `json:"latency_per_level"`
	LowModulePort   messaging.RemotePort `json:"low_module_port"`
	UpModulePort    messaging.RemotePort `json:"up_module_port"`
}
