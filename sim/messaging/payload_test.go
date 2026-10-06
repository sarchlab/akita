package messaging

import (
	"encoding/json"
	"fmt"
	"reflect"
	"strings"
	"testing"
	"unsafe"

	"github.com/sarchlab/akita/v5/sim/messaging/internal/payloadregistry"
)

type nestedTestPayload struct {
	Inner  Msg
	Values []int
	Table  map[uint8][2]string
}
type marshalOnlyPayload struct{ Value int }

func (marshalOnlyPayload) MarshalJSON() ([]byte, error) { return []byte(`{"value":1}`), nil }

type hiddenPointerPayload struct {
	Pointer *int `json:"-"`
}

func (hiddenPointerPayload) MarshalJSON() ([]byte, error) { return []byte("{}"), nil }
func (*hiddenPointerPayload) UnmarshalJSON([]byte) error  { return nil }

func init() { payloadregistry.Registry.Register(nestedTestPayload{}) }

func TestPayloadValidation(t *testing.T) {
	invalid := []any{
		&registryTestMsg{},
		struct{ P *int }{},
		struct{ Nested []struct{ Value any } }{},
		struct{ Values map[string][]chan int }{},
		struct{ F func() }{},
		struct{ P unsafe.Pointer }{},
		hiddenPointerPayload{},
		marshalOnlyPayload{},
	}
	for i, v := range invalid {
		t.Run(fmt.Sprintf("%d", i), func(t *testing.T) {
			mustPanic(t, "payload", func() {
				defineProtocol(fmt.Sprintf("test.invalid.%d", i), RoleDef{Name: "sender", Sends: []any{v}})
			})
		})
	}
	defineProtocol("test.valid.nested", RoleDef{Name: "sender", Sends: []any{nestedTestPayload{}}})
}

func TestMessageJSONPreservesNestedPayloads(t *testing.T) {
	inner := Msg{ID: 7, Src: "a", Dst: "b", Payload: registryTestMsg{Value: 42}}
	msg := Msg{ID: 8, Src: "b", Dst: "c", RspTo: 7, Payload: nestedTestPayload{Inner: inner, Values: []int{1, 2}}}
	if err := roundTripMsg(msg); err != nil {
		t.Fatal(err)
	}
	raw, err := json.Marshal(msg)
	if err != nil {
		t.Fatal(err)
	}
	var fields map[string]json.RawMessage
	if err := json.Unmarshal(raw, &fields); err != nil {
		t.Fatal(err)
	}
	if string(fields["ID"]) != "8" || len(fields["type"]) == 0 || len(fields["payload"]) == 0 {
		t.Fatalf("missing envelope fields: %s", raw)
	}
	t.Logf("nested message checkpoint: %s", raw)
	metadata := Msg{ID: 9, RspTo: 8}
	raw, err = json.Marshal(metadata)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(raw), "payload") || strings.Contains(string(raw), "type") {
		t.Fatalf("nil payload must omit codec fields: %s", raw)
	}
	restored := msg
	if err := json.Unmarshal(raw, &restored); err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(metadata, restored) {
		t.Fatalf("old payload survived decode: %#v", restored)
	}
}

func TestMessageJSONRejectsUnknownPayloadWithoutChangingReceiver(t *testing.T) {
	msg := Msg{ID: 1, Payload: registryTestMsg{Value: 2}}
	before := msg
	if err := json.Unmarshal([]byte(`{"ID":9,"type":"unknown","payload":{}}`), &msg); err == nil {
		t.Fatal("accepted unknown payload")
	}
	if !reflect.DeepEqual(msg, before) {
		t.Fatal("failed decode changed the receiver")
	}
	if _, err := json.Marshal(Msg{Payload: &registryTestMsg{}}); err == nil {
		t.Fatal("encoded unregistered pointer")
	}
}
