package sim

// A Msg is a piece of information that is transferred between components.
type Msg interface {
	Meta() *MsgMeta
}

// MsgMeta contains the meta data that is attached to every message.
type MsgMeta struct {
	ID                 string
	Src, Dst           Port
	SendTime, RecvTime VTimeInSec
	TrafficClass       int
	TrafficBytes       int
}

// Rsp is a special message that is used to indicate the completion of a
// request.
type Rsp interface {
	Msg
	GetRspTo() string
}

// GeneralRsp is a general response message that is used to indicate the
// completion of a request.
type GeneralRsp struct {
	MsgMeta

	OriginalReq Msg
}

// Meta returns the meta data of the message.
func (r *GeneralRsp) Meta() *MsgMeta {
	return &r.MsgMeta
}

// GetRspTo returns the ID of the original request.
func (r *GeneralRsp) GetRspTo() string {
	return r.OriginalReq.Meta().ID
}

// GeneralRspBuilder can build general response messages.
type GeneralRspBuilder struct {
	Src, Dst     Port
	SendTime     VTimeInSec
	TrafficClass int
	TrafficBytes int
	OriginalReq  Msg
}

// WithSrc sets the source of the general response message.
func (c GeneralRspBuilder) WithSrc(src Port) GeneralRspBuilder {
	c.Src = src
	return c
}

// WithDst sets the destination of the general response message.
func (c GeneralRspBuilder) WithDst(dst Port) GeneralRspBuilder {
	c.Dst = dst
	return c
}

// WithSendTime sets the send time of the general response message.
func (c GeneralRspBuilder) WithSendTime(sendTime VTimeInSec) GeneralRspBuilder {
	c.SendTime = sendTime
	return c
}

// WithTrafficClass sets the traffic class of the general response message.
func (c GeneralRspBuilder) WithTrafficClass(trafficClass int) GeneralRspBuilder {
	c.TrafficClass = trafficClass
	return c
}

// WithTrafficBytes sets the traffic bytes of the general response message.
func (c GeneralRspBuilder) WithTrafficBytes(trafficBytes int) GeneralRspBuilder {
	c.TrafficBytes = trafficBytes
	return c
}

// WithOriginalReq sets the original request of the general response message.
func (c GeneralRspBuilder) WithOriginalReq(originalReq Msg) GeneralRspBuilder {
	c.OriginalReq = originalReq
	return c
}

// Build creates a new general response message.
func (c GeneralRspBuilder) Build() *GeneralRsp {
	rsp := &GeneralRsp{
		MsgMeta: MsgMeta{
			Src:          c.Src,
			Dst:          c.Dst,
			SendTime:     c.SendTime,
			TrafficClass: c.TrafficClass,
			TrafficBytes: c.TrafficBytes,
			ID:           GetIDGenerator().Generate(),
		},
		OriginalReq: c.OriginalReq,
	}

	return rsp
}

// ControlMsg is a special message that is used to manipulate the states of
// some components.
type ControlMsg struct {
	MsgMeta

	Reset      bool
	Disable    bool
	Enable     bool
	ClearPorts bool
}

// Meta returns the meta data of the control message.
func (c *ControlMsg) Meta() *MsgMeta {
	return &c.MsgMeta
}

// ControlMsgBuilder can build control messages.
type ControlMsgBuilder struct {
	Src, Dst                           Port
	SendTime                           VTimeInSec
	TrafficClass                       int
	TrafficBytes                       int
	Reset, Disable, Enable, ClearPorts bool
}

// WithSrc sets the source of the control message.
func (c ControlMsgBuilder) WithSrc(src Port) ControlMsgBuilder {
	c.Src = src
	return c
}

// WithDst sets the destination of the control message.
func (c ControlMsgBuilder) WithDst(dst Port) ControlMsgBuilder {
	c.Dst = dst
	return c
}

// WithSendTime sets the send time of the control message.
func (c ControlMsgBuilder) WithSendTime(sendTime VTimeInSec) ControlMsgBuilder {
	c.SendTime = sendTime
	return c
}

// WithTrafficClass sets the traffic class of the control message.
func (c ControlMsgBuilder) WithTrafficClass(trafficClass int) ControlMsgBuilder {
	c.TrafficClass = trafficClass
	return c
}

// WithTrafficBytes sets the traffic bytes of the control message.
func (c ControlMsgBuilder) WithTrafficBytes(trafficBytes int) ControlMsgBuilder {
	c.TrafficBytes = trafficBytes
	return c
}

// WithReset sets the reset flag of the control message.
func (c ControlMsgBuilder) WithReset() ControlMsgBuilder {
	c.Reset = true
	return c
}

// WithDisable sets the disable flag of the control message.
func (c ControlMsgBuilder) WithDisable() ControlMsgBuilder {
	c.Disable = true
	return c
}

// WithEnable sets the enable flag of the control message.
func (c ControlMsgBuilder) WithEnable() ControlMsgBuilder {
	c.Enable = true
	return c
}

// WithClearPorts sets the clear ports flag of the control message.
func (c ControlMsgBuilder) WithClearPorts() ControlMsgBuilder {
	c.ClearPorts = true
	return c
}

// Build creates a new control message.
func (c ControlMsgBuilder) Build() *ControlMsg {
	msg := &ControlMsg{
		MsgMeta: MsgMeta{
			Src:          c.Src,
			Dst:          c.Dst,
			SendTime:     c.SendTime,
			TrafficClass: c.TrafficClass,
			TrafficBytes: c.TrafficBytes,
			ID:           GetIDGenerator().Generate(),
		},
		Reset:      c.Reset,
		Disable:    c.Disable,
		Enable:     c.Enable,
		ClearPorts: c.ClearPorts,
	}

	return msg
}
