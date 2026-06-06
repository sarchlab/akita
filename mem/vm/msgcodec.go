package vm

import "github.com/sarchlab/akita/v5/messaging"

// init registers the VM protocol message types so they can be serialized in
// checkpoints — for example when a TLB/MMU/address-translator port buffer holds
// a translation request or response mid-transaction. Without this, a checkpoint
// taken with VM traffic in flight would fail to load with an unknown message
// type.
func init() {
	messaging.RegisterMsg(TranslationReq{})
	messaging.RegisterMsg(TranslationRsp{})
}
