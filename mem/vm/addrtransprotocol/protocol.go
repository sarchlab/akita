// Package addrtransprotocol defines the address translation protocol: the message
// types exchanged between translation requesters (TLBs, address translators)
// and responders (TLBs, MMUs), and the protocol roles ports bind to.
package addrtransprotocol

import (
	"github.com/sarchlab/akita/v5/mem/vm"
	"github.com/sarchlab/akita/v5/sim/messaging"
)

// Protocol is the address translation protocol: requesters (TLBs, address
// translators) issue translation requests, responders (TLBs, MMUs) answer
// with the translated page. Defining the protocol registers every message
// type it carries with the checkpoint codec.
var (
	Protocol = messaging.DefineProtocol(
		messaging.RoleDef{Name: "requester",
			Sends: []any{Req{}}},
		messaging.RoleDef{Name: "responder",
			Sends: []any{Rsp{}}},
		messaging.RoleDef{Name: "notifier",
			Sends: []any{PageFault{}}},
	)
	Requester = Protocol.Role("requester")
	Responder = Protocol.Role("responder")
	Notifier  = Protocol.Role("notifier")
)

// Req is a translation request.
type Req struct {
	VAddr        uint64
	PID          vm.PID
	DeviceID     uint64
	TransLatency uint64
}

// Rsp is a translation response carrying the physical address. When Fault is
// set, the translation faulted: Page is the current mapping, which the
// requester must not use, and no TLB caches it. The requester holds the
// access until it is released (memcontrolprotocol.CmdRelease) and then
// translates it again. A Release clears only the faults whose responses
// have arrived; a response still on its way through the TLBs faults the page
// again after the Release (see PageFault).
type Rsp struct {
	Page  vm.Page
	Fault bool
}

// PageFault notifies the agent that services faults (a driver) that the
// translation of page VAddr of process PID, requested by device DeviceID,
// faulted. Page is the mapping the walk found. A responder that faults sends
// it from a notifier port; no answer is expected.
//
// The notification goes to the fault handler directly, while the fault
// response (Rsp.Fault) travels through the TLBs to the requester, so a
// Release sent on the notification can overtake the response and leave the
// access waiting for another Release. The fault handler must order the two,
// for example by holding the page (memcontrolprotocol.CmdHold) before it
// releases it: the Hold acks only after the translation in flight returns.
type PageFault struct {
	PID      vm.PID
	VAddr    uint64
	DeviceID uint64
	Page     vm.Page
}
