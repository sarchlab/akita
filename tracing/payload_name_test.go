package tracing

import (
	"github.com/sarchlab/akita/v5/mem/memprotocol"
	"github.com/sarchlab/akita/v5/messaging"
	"testing"
)

func TestMessageTraceNamesUsePayload(t *testing.T) {
	for _, tc := range []struct {
		msg  messaging.Msg
		want string
	}{
		{messaging.Msg{Payload: memprotocol.ReadReq{}}, "ReadReq"},
		{messaging.Msg{Payload: memprotocol.WriteDoneRsp{}}, "WriteDoneRsp"},
		{messaging.Msg{}, "metadata"},
	} {
		if got := msgTypeName(tc.msg); got != tc.want {
			t.Errorf("payload %T: got %q, want %q", tc.msg.Payload, got, tc.want)
		}
	}
}
