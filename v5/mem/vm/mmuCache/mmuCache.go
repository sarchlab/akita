package mmuCache

import (
	"github.com/sarchlab/akita/v5/sim"
)

const (
	mmuCacheStateEnable = "enable"
	mmuCacheStatePause  = "pause"
	mmuCacheStateDrain  = "drain"
	mmuCacheStateFlush  = "flush"
)

// Spec contains immutable configuration for the mmuCache.
type Spec struct {
	Freq            sim.Freq       `json:"freq"`
	NumBlocks       int            `json:"num_blocks"`
	NumLevels       int            `json:"num_levels"`
	PageSize        uint64         `json:"page_size"`
	Log2PageSize    uint64         `json:"log2_page_size"`
	NumReqPerCycle  int            `json:"num_req_per_cycle"`
	LatencyPerLevel uint64         `json:"latency_per_level"`
	LowModulePort   sim.RemotePort `json:"low_module_port"`
	UpModulePort    sim.RemotePort `json:"up_module_port"`
}

// State contains mutable runtime data for the mmuCache.
type State struct {
	CurrentState           string   `json:"current_state"`
	Table                  []setState `json:"table"`
	InflightFlushReqID     string         `json:"inflight_flush_req_id"`
	InflightFlushReqSrc    sim.RemotePort `json:"inflight_flush_req_src"`
	InflightFlushReqActive bool           `json:"inflight_flush_req_active"`
}


