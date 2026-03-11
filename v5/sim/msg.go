package sim

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
