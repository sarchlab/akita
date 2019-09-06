package akita

// A Msg is a piece of information that is transferred between components.
type Msg interface {
	Meta() *MsgMeta

	// All messages are simply events that can be scheduled to the receiver.
	Event
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

// Handler returns the destination component.
func (m MsgMeta) Handler() Handler {
	return m.Dst.Component()
}

// Time returns the message's event time.
func (m MsgMeta) Time() VTimeInSec {
	return m.EventTime
}
