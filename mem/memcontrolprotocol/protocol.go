package memcontrolprotocol

import (
	"github.com/sarchlab/akita/v5/mem/vm"
	"github.com/sarchlab/akita/v5/sim/messaging"
)

// Protocol is the uniform control protocol for memory agents: a requester
// (the simulation driver) issues control verbs over a component's "Control"
// port and the component responds. See mem/CONTROL_PROTOCOL.md for verb
// definitions, response timing, the support matrix, and per-component
// behavior. Defining the protocol registers the message types with the
// checkpoint codec.
var (
	Protocol = messaging.DefineProtocol(
		messaging.RoleDef{Name: "requester",
			Sends: []any{Req{}}},
		messaging.RoleDef{Name: "responder",
			Sends: []any{Rsp{}}},
	)
	Requester = Protocol.Role("requester")
	Responder = Protocol.Role("responder")
)

// Command enumerates the verbs of the uniform control protocol for memory
// agents. Zero is invalid. Every memory agent component implements its supported subset of
// these verbs over its "Control" port.
type Command int

const (
	// CmdPause stops new traffic and freezes in-flight state in place.
	// Universal verb. Ack is synchronous.
	CmdPause Command = iota + 1

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

	// CmdHold stops the accesses to the pages at Addresses (page-aligned
	// virtual addresses) of process PID: a new access to them waits, and
	// so does an access whose translation arrives after the Hold. Other
	// pages are not affected, and the component keeps processing control
	// commands while a Hold waits. Legal in any state. Conditional verb
	// (address translators). Ack is asynchronous: once no issued access to
	// the pages is outstanding and no translation of them is in flight.
	CmdHold

	// CmdRelease sends the accesses that wait on the pages at Addresses of
	// process PID for translation again, in the order they arrived. It
	// ends a Hold of the pages and also releases the accesses that wait on
	// a translation fault. Releasing a page that is not held is not an
	// error. Legal in any state. Conditional verb (address translators).
	// Ack is synchronous.
	CmdRelease
)

// Req is the unified control request for all memory agents.
type Req struct {
	Command   Command
	Addresses []uint64 // Invalidate / Flush filter (empty = all entries); Hold / Release pages.
	PID       vm.PID   // Invalidate / Flush filter (zero = all PIDs); Hold / Release process.
}

// Rsp is the unified response to a Req.
//
// Success is true when the verb was accepted and (for async verbs)
// completed. When Success is false, Error names the reason; conventional
// values include "unsupported" (the component does not implement this
// verb) and "must be paused or drained" (Invalidate/Flush issued while
// Enabled).
type Rsp struct {
	Command Command
	Success bool
	Error   string
}
