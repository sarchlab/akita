// Package vmprotocol defines the address translation protocol: the message
// types exchanged between translation requesters (TLBs, address translators)
// and responders (TLBs, MMUs), and the protocol roles ports bind to.
package vmprotocol

import (
	"github.com/sarchlab/akita/v5/mem/vm"
	"github.com/sarchlab/akita/v5/messaging"
)

// Protocol is the address translation protocol: requesters (TLBs, address
// translators) issue translation requests, responders (TLBs, MMUs) answer
// with the translated page. Defining the protocol registers every message
// type it carries with the checkpoint codec.
var (
	Protocol = messaging.DefineProtocol("vm",
		messaging.RoleDef{Name: "requester",
			Sends: []messaging.Msg{TranslationReq{}}},
		messaging.RoleDef{Name: "responder",
			Sends: []messaging.Msg{TranslationRsp{}}},
	)
	Requester = Protocol.Role("requester")
	Responder = Protocol.Role("responder")
)

// FaultProtocol carries the translation faults that an MMU reports to the
// agent that services them (a driver): the reporter sends a PageFault and
// expects no answer. The faulting translation itself is answered to its
// requester with TranslationRsp.Fault set; the agent that services the
// fault later releases the accesses that wait on it (see
// memcontrolprotocol.CmdRelease).
var (
	FaultProtocol = messaging.DefineProtocol("vm.fault",
		messaging.RoleDef{Name: "reporter",
			Sends: []messaging.Msg{PageFault{}}},
		messaging.RoleDef{Name: "handler"},
	)
	FaultReporter = FaultProtocol.Role("reporter")
	FaultHandler  = FaultProtocol.Role("handler")
)

// TranslationReq is a translation request. CanFault allows the responder to
// answer with a fault when the page is on another device than DeviceID
// (see TranslationRsp.Fault); TLBs pass it on with the misses they forward.
type TranslationReq struct {
	messaging.MsgMeta
	VAddr        uint64
	PID          vm.PID
	DeviceID     uint64
	TransLatency uint64
	CanFault     bool
}

// TranslationRsp is a translation response carrying the physical address.
// When Fault is set, the translation faulted: Page is the current mapping,
// which the requester must not use, and no TLB caches it. The requester
// waits until the fault is serviced and then translates again.
type TranslationRsp struct {
	messaging.MsgMeta
	Page  vm.Page
	Fault bool
}

// PageFault reports a translation of page VAddr of process PID, requested
// by device DeviceID, that faulted because the page is on another device.
// Page is the mapping the walk found.
type PageFault struct {
	messaging.MsgMeta
	PID      vm.PID
	VAddr    uint64
	DeviceID uint64
	Page     vm.Page
}
