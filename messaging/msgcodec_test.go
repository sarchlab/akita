package messaging

import (
	"encoding/json"
	"strings"
	"testing"
)

type registryTestMsg struct {
	MsgMeta
	Value int `json:"value"`
}

func TestMsgRegistryRoundTrip(t *testing.T) {
	RegisterMsg(registryTestMsg{})

	msg := registryTestMsg{Value: 42}
	msg.ID = 7
	msg.Src = "A"
	msg.Dst = "B"
	msg.TrafficClass = "test"

	if err := CheckRoundTrip(msg); err != nil {
		t.Fatalf("CheckRoundTrip: %v", err)
	}
}

func TestMsgRegistryUnknownType(t *testing.T) {
	_, err := msgCodec.DecodeSlice(
		json.RawMessage(`[{"type":"nonexistent.Type","payload":{}}]`))
	if err == nil || !strings.Contains(err.Error(), "unknown message type") {
		t.Fatalf("expected unknown-type error, got %v", err)
	}
}
