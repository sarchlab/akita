package mem

import (
	"github.com/sarchlab/akita/v5/mem/vm"
	"github.com/sarchlab/akita/v5/sim"
)

var accessReqByteOverhead = 12
var accessRspByteOverhead = 4
var controlMsgByteOverhead = 4

// AccessReq abstracts read and write requests sent to cache modules or memory
// controllers.
type AccessReq interface {
	sim.Msg
	GetAddress() uint64
	GetByteSize() uint64
	GetPID() vm.PID
}

// AccessRsp abstracts response messages in the memory system.
type AccessRsp interface {
	sim.Msg
}

// ReadReq is a read request sent to a memory controller.
type ReadReq struct {
	sim.MsgMeta
	Address            uint64
	AccessByteSize     uint64
	PID                vm.PID
	CanWaitForCoalesce bool
	Info               interface{} `json:"-"`
}

// GetByteSize returns the number of bytes that the request is accessing.
func (r *ReadReq) GetByteSize() uint64 {
	return r.AccessByteSize
}

// GetAddress returns the address that the request is accessing.
func (r *ReadReq) GetAddress() uint64 {
	return r.Address
}

// GetPID returns the process ID that the request is working on.
func (r *ReadReq) GetPID() vm.PID {
	return r.PID
}

// WriteReq is a write request sent to a memory controller.
type WriteReq struct {
	sim.MsgMeta
	Address            uint64
	Data               []byte
	DirtyMask          []bool
	PID                vm.PID
	CanWaitForCoalesce bool
	Info               interface{} `json:"-"`
}

// GetByteSize returns the number of bytes that the request is writing.
func (r *WriteReq) GetByteSize() uint64 {
	return uint64(len(r.Data))
}

// GetAddress returns the address that the request is accessing.
func (r *WriteReq) GetAddress() uint64 {
	return r.Address
}

// GetPID returns the PID of the read address.
func (r *WriteReq) GetPID() vm.PID {
	return r.PID
}

// DataReadyRsp is a response carrying data loaded from memory.
type DataReadyRsp struct {
	sim.MsgMeta
	Data []byte
}

// WriteDoneRsp is a response indicating a write request is completed.
type WriteDoneRsp struct {
	sim.MsgMeta
}

// ControlMsg is a control message used for managing components on the memory
// hierarchy.
type ControlMsg struct {
	sim.MsgMeta
	DiscardTransations bool
	Restart            bool
	NotifyDone         bool
	Enable             bool
	Drain              bool
	Flush              bool
	Pause              bool
	Invalid            bool
}

// ControlMsgRsp is a response to a control message.
type ControlMsgRsp struct {
	sim.MsgMeta
	Enable  bool
	Drain   bool
	Flush   bool
	Pause   bool
	Invalid bool
}
