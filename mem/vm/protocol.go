// Package vm provides the models for address translations
package vm

import (
	"github.com/sarchlab/akita/v5/messaging"
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
