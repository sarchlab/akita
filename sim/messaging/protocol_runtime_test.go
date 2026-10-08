package messaging_test

import (
	"encoding/json"
	"sync"
	"testing"

	"github.com/sarchlab/akita/v5/sim/messaging"
)

type runtimePayload struct {
	Value int
}

var runtimeProtocolOnce sync.Once

func registerRuntimeProtocol() {
	// Real protocols should be package-level declarations. This deliberately
	// registers from a test function to guard against runtime-internal checks.
	// Once preserves the one-protocol-per-package rule under go test -count.
	runtimeProtocolOnce.Do(func() {
		messaging.DefineProtocol(messaging.RoleDef{Name: "peer", Sends: []any{runtimePayload{}}})
	})
}

func TestDefineProtocolDoesNotRequireInitStack(t *testing.T) {
	registerRuntimeProtocol()
	msg := messaging.Msg{ID: 7, Payload: runtimePayload{Value: 42}}
	raw, err := json.Marshal(msg)
	if err != nil {
		t.Fatal(err)
	}
	var restored messaging.Msg
	if err := json.Unmarshal(raw, &restored); err != nil {
		t.Fatal(err)
	}
	if restored.ID != msg.ID || restored.Payload != msg.Payload {
		t.Fatalf("restored message = %#v, want %#v", restored, msg)
	}
	t.Logf("protocol registered outside initialization; restored payload %T with value 42", restored.Payload)
}

func TestPayloadRegistered(t *testing.T) {
	registerRuntimeProtocol()
	for _, tc := range []struct {
		name    string
		payload any
		want    bool
	}{
		{"nil", nil, true},
		{"registered value", runtimePayload{Value: 42}, true},
		{"unregistered value", struct{ Unregistered int }{}, false},
		{"pointer to registered value", &runtimePayload{}, false},
		{"typed nil pointer", (*runtimePayload)(nil), false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if got := messaging.PayloadRegistered(tc.payload); got != tc.want {
				t.Fatalf("PayloadRegistered(%T) = %v, want %v", tc.payload, got, tc.want)
			}
			t.Logf("payload %T: registered=%v", tc.payload, tc.want)
		})
	}
}
