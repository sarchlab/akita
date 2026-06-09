package mem

import (
	"github.com/sarchlab/akita/v5/mem/vm"
	"github.com/sarchlab/akita/v5/messaging"
)

// Protocol is the memory access protocol: requesters issue reads and writes,
// responders (caches, memory controllers) answer with data-ready and
// write-done responses. Defining the protocol registers every message type it
// carries with the checkpoint codec. The Info field on these messages is
// tagged json:"-" and is not checkpointed.
var (
	Protocol = messaging.DefineProtocol("mem",
		messaging.RoleDef{Name: "requester",
			Sends: []messaging.Msg{ReadReq{}, WriteReq{}}},
		messaging.RoleDef{Name: "responder",
			Sends: []messaging.Msg{DataReadyRsp{}, WriteDoneRsp{}}},
	)
	Requester = Protocol.Role("requester")
	Responder = Protocol.Role("responder")
)

// ControlProtocol is the uniform control protocol for memory agents: a
// requester (the simulation driver) issues control verbs over a component's
// "Control" port and the component responds. See mem/CONTROL_PROTOCOL.md.
var (
	ControlProtocol = messaging.DefineProtocol("mem.control",
		messaging.RoleDef{Name: "requester",
			Sends: []messaging.Msg{ControlReq{}}},
		messaging.RoleDef{Name: "responder",
			Sends: []messaging.Msg{ControlRsp{}}},
	)
	ControlRequester = ControlProtocol.Role("requester")
	ControlResponder = ControlProtocol.Role("responder")
)

// AccessReq abstracts read and write requests sent to cache modules or memory
// controllers.
type AccessReq interface {
	messaging.Msg
	GetAddress() uint64
	GetByteSize() uint64
	GetPID() vm.PID
}

// AccessRsp abstracts response messages in the memory system.
type AccessRsp interface {
	messaging.Msg
}

// ReadReq is a read request sent to a memory controller.
type ReadReq struct {
	messaging.MsgMeta
	Address            uint64
	AccessByteSize     uint64
	PID                vm.PID
	CanWaitForCoalesce bool
	Info               interface{} `json:"-"`
}

// GetByteSize returns the number of bytes that the request is accessing.
func (r ReadReq) GetByteSize() uint64 {
	return r.AccessByteSize
}

// GetAddress returns the address that the request is accessing.
func (r ReadReq) GetAddress() uint64 {
	return r.Address
}

// GetPID returns the process ID that the request is working on.
func (r ReadReq) GetPID() vm.PID {
	return r.PID
}

// WriteReq is a write request sent to a memory controller.
type WriteReq struct {
	messaging.MsgMeta
	Address            uint64
	Data               []byte
	DirtyMask          []bool
	PID                vm.PID
	CanWaitForCoalesce bool
	Info               interface{} `json:"-"`
}

// GetByteSize returns the number of bytes that the request is writing.
func (r WriteReq) GetByteSize() uint64 {
	return uint64(len(r.Data))
}

// GetAddress returns the address that the request is accessing.
func (r WriteReq) GetAddress() uint64 {
	return r.Address
}

// GetPID returns the PID of the read address.
func (r WriteReq) GetPID() vm.PID {
	return r.PID
}

// DataReadyRsp is a response carrying data loaded from memory.
type DataReadyRsp struct {
	messaging.MsgMeta
	Data []byte
}

// WriteDoneRsp is a response indicating a write request is completed.
type WriteDoneRsp struct {
	messaging.MsgMeta
}

// ControlCommand enumerates the verbs of the uniform control protocol for
// memory agents. Every memory agent component implements its supported
// subset of these verbs over its "Control" port. See mem/CONTROL_PROTOCOL.md
// for definitions, response timing, the support matrix, and per-component
// behavior.
type ControlCommand int

const (
	// CmdPause stops new traffic and freezes in-flight state in place.
	// Universal verb. Ack is synchronous.
	CmdPause ControlCommand = iota

	// CmdDrain stops new traffic and lets in-flight finish; ends paused.
	// Universal verb. Ack is asynchronous (on completion).
	CmdDrain

	// CmdEnable resumes processing from paused.
	// Universal verb. Ack is synchronous.
	CmdEnable

	// CmdReset hard-resets the component to its post-build state. Legal
	// from any state. Control commands are processed serially, so a Reset
	// queued behind an in-flight async verb waits for that verb to ack
	// before it runs (no preemption). Universal verb. Ack is synchronous.
	CmdReset

	// CmdInvalidate drops entries from the agent's private cache state
	// without writeback. Requires Paused or Drained. May be filtered by
	// Addresses and PID. Conditional verb (caches, TLB, MMU cache).
	// Ack is synchronous.
	CmdInvalidate

	// CmdFlush writes back dirty private state to backing memory. Clean
	// entries remain valid. Requires Paused or Drained. May be filtered
	// by Addresses and PID. Conditional verb (caches only). Ack is
	// asynchronous (on writeback completion).
	CmdFlush
)

// ControlReq is the unified control request for all memory agents.
type ControlReq struct {
	messaging.MsgMeta
	Command   ControlCommand
	Addresses []uint64 // Invalidate / Flush filter; empty = all entries.
	PID       vm.PID   // Invalidate / Flush filter; zero = all PIDs.
}

// ControlRsp is the unified response to a ControlReq.
//
// Success is true when the verb was accepted and (for async verbs)
// completed. When Success is false, Error names the reason; conventional
// values include "unsupported" (the component does not implement this
// verb) and "must be paused or drained" (Invalidate/Flush issued while
// Enabled).
type ControlRsp struct {
	messaging.MsgMeta
	Command ControlCommand
	Success bool
	Error   string
}
