package vm_test

import (
	"testing"

	"github.com/sarchlab/akita/v5/mem/vm"
	"github.com/sarchlab/akita/v5/messaging"
)

// TestVMProtocolMessagesRoundTrip confirms the VM translation messages are
// registered by default, so a checkpoint taken with TLB/MMU traffic in flight
// can be loaded back.
func TestVMProtocolMessagesRoundTrip(t *testing.T) {
	req := vm.TranslationReq{VAddr: 0x1000, PID: 1, DeviceID: 2}
	req.ID = 5
	rsp := vm.TranslationRsp{Page: vm.Page{PID: 1, VAddr: 0x1000, PAddr: 0x4000}}
	rsp.ID = 6

	for _, msg := range []messaging.Msg{req, rsp} {
		if err := messaging.CheckRoundTrip(msg); err != nil {
			t.Fatalf("round trip %T (is it registered and lossless?): %v", msg, err)
		}
	}
}
