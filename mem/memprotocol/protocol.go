// Package memprotocol defines the memory access protocol: the message types
// exchanged between memory requesters (cores, caches issuing fills) and
// responders (caches, memory controllers), and the protocol roles ports bind
// to.
package memprotocol

import (
	"github.com/sarchlab/akita/v5/mem/vm"
	"github.com/sarchlab/akita/v5/sim/messaging"
)

// Protocol is the memory access protocol: requesters issue reads and writes,
// responders (caches, memory controllers) answer with data-ready and
// write-done responses. Defining the protocol registers every message type it
// carries with the checkpoint codec.
var (
	Protocol = messaging.DefineProtocol(
		messaging.RoleDef{Name: "requester",
			Sends: []any{ReadReq{}, WriteReq{}}},
		messaging.RoleDef{Name: "responder",
			Sends: []any{DataReadyRsp{}, WriteDoneRsp{}}},
	)
	Requester = Protocol.Role("requester")
	Responder = Protocol.Role("responder")
)

// AccessReq abstracts read and write requests sent to cache modules or memory
// controllers.
type AccessReq interface {
	AccessAddress() uint64
	AccessPID() vm.PID
}

// ReadReq is a read request sent to a memory controller.
type ReadReq struct {
	Address            uint64
	AccessByteSize     uint64
	PID                vm.PID
	CanWaitForCoalesce bool
}

// AccessAddress returns the address that the request is accessing.
func (r ReadReq) AccessAddress() uint64 {
	return r.Address
}

// AccessPID returns the process ID that the request is working on.
func (r ReadReq) AccessPID() vm.PID {
	return r.PID
}

// WriteReq is a write request sent to a memory controller.
type WriteReq struct {
	Address            uint64
	Data               []byte
	DirtyMask          []bool
	PID                vm.PID
	CanWaitForCoalesce bool
}

// AccessAddress returns the address that the request is accessing.
func (r WriteReq) AccessAddress() uint64 {
	return r.Address
}

// AccessPID returns the process ID that the request is working on.
func (r WriteReq) AccessPID() vm.PID {
	return r.PID
}

// DataReadyRsp is a response carrying data loaded from memory.
type DataReadyRsp struct {
	Data []byte
}

// WriteDoneRsp is a response indicating a write request is completed.
type WriteDoneRsp struct {
}
