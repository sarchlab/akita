package simplebankedmemory

import (
	"github.com/sarchlab/akita/v5/mem"
	"github.com/sarchlab/akita/v5/queueing"
)

// bankPipelineItemState is a serializable representation of a pipeline item.
type bankPipelineItemState struct {
	IsRead    bool         `json:"is_read"`
	ReadMsg   mem.ReadReq  `json:"read_msg"`
	WriteMsg  mem.WriteReq `json:"write_msg"`
	Committed bool         `json:"committed"`
	ReadData  []byte       `json:"read_data"`
}

// bankState captures one bank pipeline + buffer contents.
type bankState struct {
	Pipeline        queueing.Pipeline[bankPipelineItemState] `json:"pipeline"`
	PostPipelineBuf []bankPipelineItemState                  `json:"post_pipeline_buf"`
}

// State contains mutable runtime data for the simple banked memory.
type State struct {
	Banks []bankState `json:"banks"`
}
