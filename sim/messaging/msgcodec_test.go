package messaging

import (
	"encoding/json"
	"fmt"
	"reflect"
	"strings"
	"testing"

	"github.com/sarchlab/akita/v5/sim/messaging/internal/payloadregistry"
)

type registryTestMsg struct {
	Value int `json:"value"`
}

func TestMsgRegistryRoundTrip(t *testing.T) {
	payloadregistry.Registry.Register(registryTestMsg{})

	msg := Msg{Payload: registryTestMsg{Value: 42}}
	msg.ID = 7
	msg.Src = "A"
	msg.Dst = "B"
	msg.TrafficClass = "test"

	if err := roundTripMsg(msg); err != nil {
		t.Fatalf("CheckRoundTrip: %v", err)
	}
}

func TestMsgRegistryUnknownType(t *testing.T) {
	_, err := payloadregistry.Registry.DecodeSlice(
		json.RawMessage(`[{"type":"nonexistent.Type","payload":{}}]`))
	if err == nil || !strings.Contains(err.Error(), "unknown payload type") {
		t.Fatalf("expected unknown-type error, got %v", err)
	}
}

func init() { payloadregistry.Registry.Register(registryTestMsg{}) }

func roundTripMsg(msg Msg) error {
	raw, err := json.Marshal(msg)
	if err != nil {
		return err
	}
	var restored Msg
	if err := json.Unmarshal(raw, &restored); err != nil {
		return err
	}
	if !reflect.DeepEqual(msg, restored) {
		return fmt.Errorf("round trip: got %#v, want %#v", restored, msg)
	}
	return nil
}
