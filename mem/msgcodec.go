package mem

import "github.com/sarchlab/akita/v5/messaging"

// init registers the memory protocol message types so they can be serialized in
// checkpoints — for example when a port buffer holds them mid-transaction. The
// Info field on these messages is tagged json:"-" and is not checkpointed.
func init() {
	messaging.RegisterMsg(&ReadReq{})
	messaging.RegisterMsg(&WriteReq{})
	messaging.RegisterMsg(&DataReadyRsp{})
	messaging.RegisterMsg(&WriteDoneRsp{})
	messaging.RegisterMsg(&ControlReq{})
	messaging.RegisterMsg(&ControlRsp{})
}
