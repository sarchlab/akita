package mem_test

import (
	"testing"

	"github.com/sarchlab/akita/v5/mem"
	"github.com/sarchlab/akita/v5/messaging"
)

func TestProtocolMessagesRoundTrip(t *testing.T) {
	read := mem.ReadReq{Address: 0x100, AccessByteSize: 64}
	read.ID = 5
	write := mem.WriteReq{Address: 0x200, Data: []byte{1, 2, 3}}
	write.ID = 6

	dataReady := mem.DataReadyRsp{}
	dataReady.ID = 7
	writeDone := mem.WriteDoneRsp{}
	writeDone.ID = 8
	ctrlReq := mem.ControlReq{}
	ctrlReq.ID = 9
	ctrlRsp := mem.ControlRsp{}
	ctrlRsp.ID = 10

	// CheckRoundTrip encodes, decodes, and compares for equality, so it covers
	// both "is this type registered?" and "does every field survive?".
	for _, msg := range []messaging.Msg{
		read, write, dataReady, writeDone, ctrlReq, ctrlRsp,
	} {
		if err := messaging.CheckRoundTrip(msg); err != nil {
			t.Fatalf("round trip %T (is it registered and lossless?): %v", msg, err)
		}
	}
}
