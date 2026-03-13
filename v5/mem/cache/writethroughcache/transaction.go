package writethroughcache

import (
	"github.com/sarchlab/akita/v5/mem/vm"
	"github.com/sarchlab/akita/v5/sim"
)

type bankActionType int

const (
	bankActionInvalid bankActionType = iota
	bankActionReadHit
	bankActionWrite
	bankActionWriteFetched
)

// transactionState is the canonical transaction type for the writethroughcache.
// All fields are directly JSON-serializable (no pointers to message types).
type transactionState struct {
	ID string `json:"id"`

	// Read request fields (flattened from *mem.ReadReq)
	HasRead            bool        `json:"has_read"`
	ReadMeta           sim.MsgMeta `json:"read_meta"`
	ReadAddress        uint64      `json:"read_address"`
	ReadAccessByteSize uint64      `json:"read_access_byte_size"`
	ReadPID            vm.PID      `json:"read_pid"`

	// ReadToBottom fields (flattened from *mem.ReadReq)
	HasReadToBottom  bool        `json:"has_read_to_bottom"`
	ReadToBottomMeta sim.MsgMeta `json:"read_to_bottom_meta"`
	ReadToBottomPID  vm.PID      `json:"read_to_bottom_pid"`

	// Write request fields (flattened from *mem.WriteReq)
	HasWrite       bool        `json:"has_write"`
	WriteMeta      sim.MsgMeta `json:"write_meta"`
	WriteAddress   uint64      `json:"write_address"`
	WriteData      []byte      `json:"write_data"`
	WriteDirtyMask []bool      `json:"write_dirty_mask"`
	WritePID       vm.PID      `json:"write_pid"`

	// WriteToBottom fields (flattened from *mem.WriteReq)
	HasWriteToBottom       bool        `json:"has_write_to_bottom"`
	WriteToBottomMeta      sim.MsgMeta `json:"write_to_bottom_meta"`
	WriteToBottomPID       vm.PID      `json:"write_to_bottom_pid"`
	WriteToBottomData      []byte      `json:"write_to_bottom_data"`
	WriteToBottomDirtyMask []bool      `json:"write_to_bottom_dirty_mask"`

	// Pre-coalesce transaction indices (absolute indices into State.Transactions)
	PreCoalesceTransIdxs []int `json:"pre_coalesce_trans_idxs"`

	BankAction            bankActionType `json:"bank_action"`
	BlockSetID            int            `json:"block_set_id"`
	BlockWayID            int            `json:"block_way_id"`
	HasBlock              bool           `json:"has_block"`
	Data                  []byte         `json:"data"`
	WriteFetchedDirtyMask []bool         `json:"write_fetched_dirty_mask"`

	FetchAndWrite   bool `json:"fetch_and_write"`
	Done            bool `json:"done"`
	BottomWriteDone bool `json:"bottom_write_done"`
	BankDone        bool `json:"bank_done"`

	// Removed marks a post-coalesce transaction that has been completed
	// and removed from active processing.
	Removed bool `json:"removed"`
}

func (t *transactionState) Address() uint64 {
	if t.HasRead {
		return t.ReadAddress
	}

	return t.WriteAddress
}

func (t *transactionState) PID() vm.PID {
	if t.HasRead {
		return t.ReadPID
	}

	return t.WritePID
}
