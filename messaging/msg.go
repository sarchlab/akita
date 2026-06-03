package messaging

// Msg is the interface for all messages transferred between components.
//
// Messages are conventionally constructed as pointer values (e.g.
// `&mem.ReadReq{...}`) because Meta() returns *MsgMeta to give a stable
// identity that survives interface boxing. Even so, callers must treat a
// message as immutable once it has been handed to a port: every field of
// MsgMeta is set at construction and never reassigned in flight.
type Msg interface {
	Meta() *MsgMeta
}

// MsgMeta contains routing and identification metadata. All fields are set at
// construction time and must not change once the message is in flight.
type MsgMeta struct {
	ID           uint64
	Src, Dst     RemotePort
	TrafficClass string
	TrafficBytes int
	RspTo        uint64
}

// Meta returns the message metadata.
func (m *MsgMeta) Meta() *MsgMeta { return m }

// IsRsp returns true if this message is a response to another message.
func (m *MsgMeta) IsRsp() bool { return m.RspTo != 0 }
