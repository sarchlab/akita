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
	// without writeback. A running data cache accepts an address filter (Range
	// or nonempty Addresses) and replies after processing those lines. In a paused agent
	// Invalidate is synchronous. PID zero matches every address space.
	CmdInvalidate

	// CmdFlush writes back dirty private state to backing memory. Clean
	// entries remain valid. Running writeback caches require an address
	// filter (Range or nonempty Addresses). Other supported Flush operations require Pause or
	// Drain. The ACK follows completion of matching writebacks.
	CmdFlush

	// CmdHold stops the accesses to the pages that Range intersects, or to the
	// pages at Addresses (virtual addresses), of process PID: a new access to them waits, and
	// so does an access whose translation arrives after the Hold. Other
	// pages are not affected, and the component keeps processing control
	// commands while a Hold waits. Legal in any state. Conditional verb
	// (address translators). Ack is asynchronous: once no issued access to
	// the pages is outstanding and no translation of them is in flight.
	CmdHold

	// CmdRelease sends the accesses that wait on the pages of Range or Addresses of
	// process PID for translation again, in the order they arrived. It
	// ends a Hold of the pages and also releases the accesses that wait on
	// a translation fault. Releasing a page that is not held is not an
	// error. Legal in any state. Conditional verb (address translators).
	// Ack is synchronous.
	CmdRelease
)

// AddressRange is the half-open byte interval [Start, End). It must not be
// empty: End is greater than Start. A range selects every cache line or page
// that it intersects, so its endpoints need no alignment. End is exclusive,
// so a range cannot include the byte at the highest address.
type AddressRange struct {
	Start uint64
	End   uint64
}

// Bounds returns the addresses of the first and the last unit that r
// intersects, where a unit is an aligned block of 1<<log2UnitSize bytes (a
// cache line or a page). Both bounds are inclusive, so they do not overflow.
// r must be valid.
func (r AddressRange) Bounds(log2UnitSize uint64) (first, last uint64) {
	return r.Start >> log2UnitSize << log2UnitSize,
		(r.End - 1) >> log2UnitSize << log2UnitSize
}

// Req is the unified control request for all memory agents.
//
// Range (with HasRange) and a nonempty Addresses are alternatives. A receiver
// that consumes the addresses of the verb rejects a request with both, a
// request with an empty or reversed Range, and a request that sets Range
// without HasRange; see AddressingError. A request with neither keeps the
// meaning of an empty Addresses.
type Req struct {
	Command Command

	// Addresses is the Invalidate / Flush filter (empty = all entries) and
	// the Hold / Release pages.
	//
	// Deprecated: use HasRange and Range. Akita v6 will address by range
	// only and remove Addresses.
	Addresses []uint64

	PID vm.PID // Invalidate / Flush filter (zero = all PIDs); Hold / Release process.

	// HasRange selects Range as the Invalidate / Flush filter and the Hold /
	// Release pages: every line or page that Range intersects. Range must
	// be the zero value when HasRange is false.
	HasRange bool
	Range    AddressRange
}

// HasAddressFilter reports whether the request names addresses, by Range or
// by a nonempty Addresses.
func (r Req) HasAddressFilter() bool {
	return r.HasRange || len(r.Addresses) > 0
}

// AddressingError returns the Rsp.Error for a request whose addressing is
// not valid, or an empty string. A receiver checks it for the verbs whose
// addresses it consumes, before any other legality check of the verb.
func (r Req) AddressingError() string {
	if !r.HasRange {
		if r.Range != (AddressRange{}) {
			return ErrInvalidRange
		}
		return ""
	}
	if r.Range.End <= r.Range.Start {
		return ErrInvalidRange
	}
	if len(r.Addresses) > 0 {
		return ErrAmbiguousAddressing
	}
	return ""
}

// Rsp is the unified response to a Req.
//
// Success is true when the verb was accepted and (for async verbs)
// completed. When Success is false, Error names the reason; conventional
// values include "unsupported" (the component does not implement this
// verb) and "must be paused or drained" (the requested maintenance
// requires Pause or Drain).
type Rsp struct {
	Command Command
	Success bool
	Error   string
}
