package twowaybuffered

import (
	"fmt"
	"testing"

	"github.com/sarchlab/akita/v5/sim/messaging"
	"github.com/stretchr/testify/require"
)

type registryTestMsg struct {
	Value int `json:"value"`
}

var _ = messaging.DefineProtocol(messaging.RoleDef{
	Name: "sender", Sends: []any{registryTestMsg{}, benchmarkPayload{}},
})

func TestSendValidatesPayloadBeforeBuffering(t *testing.T) {
	p := NewPort("src", 1, 1)
	p.SetConnection(&benchmarkConnection{})
	for _, v := range []any{&registryTestMsg{}, struct{ Unregistered int }{}} {
		require.PanicsWithValue(t,
			fmt.Sprintf("messaging: unregistered payload type %T; register a value through DefineProtocol", v),
			func() { p.Send(messaging.Msg{Src: "src", Dst: "dst", Payload: v}) })
		if p.NumOutgoing() != 0 {
			t.Fatal("invalid payload was buffered")
		}
	}
	p.Send(messaging.Msg{Src: "src", Dst: "dst"})
	if msg, ok := p.PeekOutgoing(); !ok || msg.Payload != nil {
		t.Fatalf("nil payload was not accepted: %#v, %v", msg, ok)
	}
}
