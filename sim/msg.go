package sim

import "reflect"

// A Msg is a piece of information that is transferred between components.
type Msg interface {
	Meta() *MsgMeta
	Clone() Msg
	// GenerateRsp() Rsp
}

// MsgMeta contains the meta data that is attached to every message.
type MsgMeta struct {
	ID           string
	Src, Dst     RemotePort
	TrafficClass string
	TrafficBytes int
}

// Rsp is a special message that is used to indicate the completion of a
// request.
type Rsp interface {
	Msg
	GetRspTo() string
}

type Request interface {
	Msg
	GenerateRsp() Rsp
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

// Clone returns cloned GeneralRsp with different ID
func (r *GeneralRsp) Clone() Msg {
	cloneMsg := *r
	cloneMsg.ID = GetIDGenerator().Generate()

	return &cloneMsg
}

// GetRspTo returns the ID of the original request.
func (r *GeneralRsp) GetRspTo() string {
	return r.OriginalReq.Meta().ID
}

// GeneralRspBuilder can build general response messages.
type GeneralRspBuilder struct {
	Src, Dst     RemotePort
	TrafficBytes int
	OriginalReq  Msg
}

// WithSrc sets the source of the general response message.
func (c GeneralRspBuilder) WithSrc(src RemotePort) GeneralRspBuilder {
	c.Src = src
	return c
}

// WithDst sets the destination of the general response message.
func (c GeneralRspBuilder) WithDst(dst RemotePort) GeneralRspBuilder {
	c.Dst = dst
	return c
}

// WithTrafficBytes sets the traffic bytes of the general response message.
func (c GeneralRspBuilder) WithTrafficBytes(
	trafficBytes int,
) GeneralRspBuilder {
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
			TrafficClass: reflect.TypeOf(GeneralRsp{}).String(),
			TrafficBytes: c.TrafficBytes,
			ID:           GetIDGenerator().Generate(),
		},
		OriginalReq: c.OriginalReq,
	}

	return rsp
}
