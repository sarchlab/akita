package sim

import "fmt"

// Msg is a piece of information transferred between components.
type Msg struct {
	MsgMeta
	RspTo   string
	Payload any
}

// MsgMeta contains routing and identification metadata.
type MsgMeta struct {
	ID           string
	Src, Dst     RemotePort
	TrafficClass string
	TrafficBytes int
}

// IsRsp returns true if this message is a response to another message.
func (m *Msg) IsRsp() bool { return m.RspTo != "" }

// Clone returns a copy of the message with a new ID.
func (m *Msg) Clone() *Msg {
	clone := *m
	clone.ID = GetIDGenerator().Generate()
	return &clone
}

// MsgPayload extracts the payload as a specific type, panicking if the type
// does not match.
func MsgPayload[T any](msg *Msg) *T {
	p, ok := msg.Payload.(*T)
	if !ok {
		panic(fmt.Sprintf("msg payload: want %T, got %T", (*T)(nil), msg.Payload))
	}
	return p
}

// TryMsgPayload extracts the payload as a specific type, returning false if the
// type does not match.
func TryMsgPayload[T any](msg *Msg) (*T, bool) {
	p, ok := msg.Payload.(*T)
	return p, ok
}
