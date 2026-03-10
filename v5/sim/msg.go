package sim

import "fmt"

// Msg is the interface for all messages transferred between components.
type Msg interface {
	Meta() *MsgMeta
}

// MsgMeta contains routing and identification metadata.
type MsgMeta struct {
	ID           string
	Src, Dst     RemotePort
	TrafficClass string
	TrafficBytes int
	RspTo        string
}

// Meta returns the message metadata.
func (m *MsgMeta) Meta() *MsgMeta { return m }

// IsRsp returns true if this message is a response to another message.
func (m *MsgMeta) IsRsp() bool { return m.RspTo != "" }

// GenericMsg is a piece of information transferred between components.
type GenericMsg struct {
	MsgMeta
	Payload any
}

// Clone returns a copy of the message with a new ID.
func (m *GenericMsg) Clone() *GenericMsg {
	clone := *m
	clone.ID = GetIDGenerator().Generate()
	return &clone
}

// MsgPayload extracts the payload as a specific type, panicking if the type
// does not match.
func MsgPayload[T any](msg *GenericMsg) *T {
	p, ok := msg.Payload.(*T)
	if !ok {
		panic(fmt.Sprintf("msg payload: want %T, got %T", (*T)(nil), msg.Payload))
	}
	return p
}

// TryMsgPayload extracts the payload as a specific type, returning false if the
// type does not match.
func TryMsgPayload[T any](msg *GenericMsg) (*T, bool) {
	p, ok := msg.Payload.(*T)
	return p, ok
}
