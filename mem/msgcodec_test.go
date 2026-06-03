package mem_test

import (
	"testing"

	"github.com/sarchlab/akita/v5/mem"
	"github.com/sarchlab/akita/v5/messaging"
)

func TestProtocolMessagesRoundTrip(t *testing.T) {
	read := &mem.ReadReq{Address: 0x100, AccessByteSize: 64}
	read.ID = 5
	write := &mem.WriteReq{Address: 0x200, Data: []byte{1, 2, 3}}
	write.ID = 6

	dataReady := &mem.DataReadyRsp{}
	dataReady.ID = 7
	writeDone := &mem.WriteDoneRsp{}
	writeDone.ID = 8
	ctrlReq := &mem.ControlReq{}
	ctrlReq.ID = 9
	ctrlRsp := &mem.ControlRsp{}
	ctrlRsp.ID = 10

	for _, msg := range []messaging.Msg{
		read, write, dataReady, writeDone, ctrlReq, ctrlRsp,
	} {
		tp, err := messaging.EncodeMsg(msg)
		if err != nil {
			t.Fatalf("encode %T: %v", msg, err)
		}
		got, err := messaging.DecodeMsg(tp)
		if err != nil {
			t.Fatalf("decode %T (is it registered?): %v", msg, err)
		}
		if got.Meta().ID != msg.Meta().ID {
			t.Fatalf("%T: ID = %d, want %d", msg, got.Meta().ID, msg.Meta().ID)
		}
	}

	// A concrete field survives the round trip.
	tp, _ := messaging.EncodeMsg(read)
	got, _ := messaging.DecodeMsg(tp)
	if got.(*mem.ReadReq).Address != 0x100 {
		t.Fatalf("ReadReq.Address not preserved: %+v", got)
	}
}
