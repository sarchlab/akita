package akita

// A Msg is a piece of information that is transferred between components.
type Msg interface {
	Meta() *MsgMeta
}

// MsgMeta contains the meta data that is attached to every message.
type MsgMeta struct {
	ID                 string
	Src, Dst           Port
	SendTime, RecvTime VTimeInSec
	EventTime          VTimeInSec
	TrafficClass       int
	TrafficBytes       int
}

// IsSecondary always returns true. Message-based events are all primary events.
func (m MsgMeta) IsSecondary() bool {
	return false
}
