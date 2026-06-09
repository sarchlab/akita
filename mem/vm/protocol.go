// Package vm provides the models for address translations
package vm

import (
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

// TranslationReq is a translation request.
type TranslationReq struct {
	messaging.MsgMeta
	VAddr        uint64
	PID          PID
	DeviceID     uint64
	TransLatency uint64
}

// TranslationRsp is a translation response carrying the physical address.
type TranslationRsp struct {
	messaging.MsgMeta
	Page Page
}
