// Package vmprotocol defines the address translation protocol: the message
// types exchanged between translation requesters (TLBs, address translators)
// and responders (TLBs, MMUs), and the protocol roles ports bind to.
package vmprotocol

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
			Sends: []any{TranslationReq{}}},
		messaging.RoleDef{Name: "responder",
			Sends: []any{TranslationRsp{}}},
	)
	Requester = Protocol.Role("requester")
	Responder = Protocol.Role("responder")
)

// TranslationReq is a translation request.
type TranslationReq struct {
	VAddr        uint64
	PID          vm.PID
	DeviceID     uint64
	TransLatency uint64
}

// TranslationRsp is a translation response carrying the physical address.
type TranslationRsp struct {
	Page vm.Page
}
