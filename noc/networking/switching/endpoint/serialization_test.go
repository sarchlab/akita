package endpoint

import (
	"encoding/json"
	"reflect"
	"testing"

	"github.com/sarchlab/akita/v5/messaging"
	"github.com/sarchlab/akita/v5/noc/packetization"
)

// fakeEndpointMsg is a registered concrete message used to verify that the
// endpoint's State round-trips the concrete messages it now carries.
type fakeEndpointMsg struct {
	messaging.MsgMeta
	Data string `json:"data"`
}

func init() {
	messaging.RegisterMsg(fakeEndpointMsg{})
}

func TestMsgHolderRoundTrips(t *testing.T) {
	t.Run("with message", func(t *testing.T) {
		h := msgHolder{Msg: fakeEndpointMsg{
			MsgMeta: messaging.MsgMeta{ID: 5, Src: "a", Dst: "b"},
			Data:    "payload",
		}}

		data, err := json.Marshal(h)
		if err != nil {
			t.Fatalf("marshal: %v", err)
		}

		var got msgHolder
		if err := json.Unmarshal(data, &got); err != nil {
			t.Fatalf("unmarshal: %v", err)
		}

		if !reflect.DeepEqual(h, got) {
			t.Fatalf("round trip mismatch:\n got %+v\nwant %+v", got, h)
		}
	})

	t.Run("nil message", func(t *testing.T) {
		data, err := json.Marshal(msgHolder{})
		if err != nil {
			t.Fatalf("marshal: %v", err)
		}

		var got msgHolder
		if err := json.Unmarshal(data, &got); err != nil {
			t.Fatalf("unmarshal: %v", err)
		}

		if got.Msg != nil {
			t.Fatalf("expected nil message, got %+v", got.Msg)
		}
	})
}

// TestStateRoundTrips checkpoints a populated endpoint State, exercising every
// message-bearing buffer: an outgoing message, an in-flight flit carrying its
// payload, a partially-assembled record whose payload flit has not yet arrived
// (nil payload), and a fully-assembled message ready for delivery.
func TestStateRoundTrips(t *testing.T) {
	msg := fakeEndpointMsg{
		MsgMeta: messaging.MsgMeta{ID: 42, Src: "dev", Dst: "peer", TrafficBytes: 64},
		Data:    "hello",
	}

	state := State{
		MsgOutBuf: []msgHolder{{Msg: msg}},
		FlitsToSend: []packetization.Flit{{
			MsgMeta:      messaging.MsgMeta{ID: 1, Src: "ep", Dst: "sw"},
			SeqID:        0,
			NumFlitInMsg: 1,
			Msg:          msg.Meta(),
			Payload:      msg,
		}},
		AssemblingMsgs: []assemblingMsgState{{
			MsgID:           7,
			NumFlitRequired: 2,
			NumFlitArrived:  1,
			Payload:         msgHolder{}, // payload-bearing flit not yet arrived
		}},
		AssembledMsgs: []msgHolder{{Msg: msg}},
	}

	data, err := json.Marshal(state)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}

	var got State
	if err := json.Unmarshal(data, &got); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}

	if !reflect.DeepEqual(state, got) {
		t.Fatalf("round trip mismatch:\n got %+v\nwant %+v", got, state)
	}
}
