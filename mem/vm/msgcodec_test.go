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
		tp, err := messaging.EncodeMsg(msg)
		if err != nil {
			t.Fatalf("EncodeMsg %T: %v", msg, err)
		}
		got, err := messaging.DecodeMsg(tp)
		if err != nil {
			t.Fatalf("DecodeMsg %T (is it registered?): %v", msg, err)
		}
		if got.Meta().ID != msg.Meta().ID {
			t.Fatalf("%T: ID = %d, want %d", msg, got.Meta().ID, msg.Meta().ID)
		}
	}

	// A concrete field survives the round trip.
	tp, _ := messaging.EncodeMsg(rsp)
	got, _ := messaging.DecodeMsg(tp)
	if got.(vm.TranslationRsp).Page.PAddr != 0x4000 {
		t.Fatalf("TranslationRsp.Page.PAddr not preserved: %+v", got)
	}
}
