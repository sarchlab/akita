package simplebankedmemory

import "github.com/sarchlab/akita/v5/sim"

// Spec contains immutable configuration for the simple banked memory.
type Spec struct {
	Freq                           sim.Freq `json:"freq"`
	NumBanks                       int      `json:"num_banks"`
	BankPipelineWidth              int    `json:"bank_pipeline_width"`
	BankPipelineDepth              int    `json:"bank_pipeline_depth"`
	StageLatency                   int    `json:"stage_latency"`
	PostPipelineBufSize            int    `json:"post_pipeline_buf_size"`
	BankSelectorKind               string `json:"bank_selector_kind"`
	BankSelectorLog2InterleaveSize uint64 `json:"bank_selector_log2_interleave_size"`
	AddrConvKind                   string `json:"addr_conv_kind"`
	AddrInterleavingSize           uint64 `json:"addr_interleaving_size"`
	AddrTotalNumOfElements         int    `json:"addr_total_num_of_elements"`
	AddrCurrentElementIndex        int    `json:"addr_current_element_index"`
	AddrOffset                     uint64 `json:"addr_offset"`
	StorageRef                     string `json:"storage_ref"`
}
