package messaging

import (
	"strings"
	"testing"
)

type registryTestMsg struct {
	MsgMeta
	Value int `json:"value"`
}

func TestMsgRegistryRoundTrip(t *testing.T) {
	RegisterMsg(&registryTestMsg{})

	msg := &registryTestMsg{Value: 42}
	msg.ID = 7
	msg.Src = "A"
	msg.Dst = "B"
	msg.TrafficClass = "test"

	tp, err := EncodeMsg(msg)
	if err != nil {
		t.Fatalf("EncodeMsg: %v", err)
	}
	if tp.Type != "*messaging.registryTestMsg" {
		t.Fatalf("type tag = %q", tp.Type)
	}

	got, err := DecodeMsg(tp)
	if err != nil {
		t.Fatalf("DecodeMsg: %v", err)
	}

	gm, ok := got.(*registryTestMsg)
	if !ok {
		t.Fatalf("decoded type = %T, want *registryTestMsg", got)
	}
	if gm.Value != 42 || gm.ID != 7 || gm.Src != "A" || gm.Dst != "B" ||
		gm.TrafficClass != "test" {
		t.Fatalf("decoded message mismatch: %+v", gm)
	}
}

func TestMsgRegistryUnknownType(t *testing.T) {
	_, err := DecodeMsg(TypedPayload{Type: "*nonexistent.Type", Payload: []byte("{}")})
	if err == nil || !strings.Contains(err.Error(), "unknown message type") {
		t.Fatalf("expected unknown-type error, got %v", err)
	}
}

// valueMsg implements Msg with a value receiver, so a value of it satisfies Msg
// without being a pointer — used only to exercise RegisterMsg's guard.
type valueMsg struct{ m MsgMeta }

func (v valueMsg) Meta() *MsgMeta { return &v.m }

func TestRegisterMsgRejectsNonPointer(t *testing.T) {
	defer func() {
		if recover() == nil {
			t.Fatalf("expected panic registering a non-pointer message")
		}
	}()
	RegisterMsg(valueMsg{})
}
