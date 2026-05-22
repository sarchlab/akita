package memaccessagent

import "github.com/sarchlab/akita/v5/mem"

// State contains the mutable runtime data for the MemAccessAgent.
type State struct {
	WriteLeft       int                     `json:"write_left"`
	ReadLeft        int                     `json:"read_left"`
	KnownMemValue   map[uint64][]uint32     `json:"known_mem_value"`
	PendingReadReq  map[uint64]mem.ReadReq  `json:"pending_read_req"`
	PendingWriteReq map[uint64]mem.WriteReq `json:"pending_write_req"`
}
