package packetization

import (
	"encoding/json"
	"reflect"
	"testing"

	"github.com/sarchlab/akita/v5/messaging"
)

// fakePayload is a registered concrete message used to verify that a flit's
// carried Payload survives JSON round-tripping through the message codec, the
// way it must when a checkpoint captures a flit in a port buffer or switch
// State.
type fakePayload struct {
	messaging.MsgMeta
	Data string `json:"data"`
}

func init() {
	messaging.RegisterMsg(fakePayload{})
}

func TestFlitWithPayloadRoundTrips(t *testing.T) {
	original := Flit{
		MsgMeta:      messaging.MsgMeta{ID: 7, Src: "ep0", Dst: "sw0"},
		SeqID:        3,
		NumFlitInMsg: 4,
		Msg:          messaging.MsgMeta{ID: 42, Src: "a", Dst: "b", TrafficBytes: 128},
		Payload: fakePayload{
			MsgMeta: messaging.MsgMeta{ID: 42, Src: "a", Dst: "b", TrafficBytes: 128},
			Data:    "hello",
		},
	}

	data, err := json.Marshal(original)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}

	var restored Flit
	if err := json.Unmarshal(data, &restored); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}

	if !reflect.DeepEqual(original, restored) {
		t.Fatalf("round trip mismatch:\n got %+v\nwant %+v", restored, original)
	}

	if _, ok := restored.Payload.(fakePayload); !ok {
		t.Fatalf("payload concrete type not preserved: got %T", restored.Payload)
	}
}

func TestFlitWithoutPayloadRoundTrips(t *testing.T) {
	original := Flit{
		MsgMeta:      messaging.MsgMeta{ID: 1, Src: "ep0", Dst: "sw0"},
		SeqID:        0,
		NumFlitInMsg: 4,
		Msg:          messaging.MsgMeta{ID: 42, Src: "a", Dst: "b"},
	}

	data, err := json.Marshal(original)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}

	var restored Flit
	if err := json.Unmarshal(data, &restored); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}

	if restored.Payload != nil {
		t.Fatalf("expected nil payload, got %+v", restored.Payload)
	}

	if !reflect.DeepEqual(original, restored) {
		t.Fatalf("round trip mismatch:\n got %+v\nwant %+v", restored, original)
	}
}
