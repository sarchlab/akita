package modeling

import (
	"github.com/sarchlab/akita/v4/sim/id"
)

// A Msg is a piece of information that is transferred between components.
type Msg interface {
	Meta() MsgMeta
	Clone() Msg
}

// MsgMeta contains the meta data that is attached to every message.
type MsgMeta struct {
	ID           string
	Src, Dst     RemotePort
	TrafficClass int
	TrafficBytes int
}

// Req is a request message.
type Req interface {
	Msg
	GenerateRsp() Rsp
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

	RspTo string
}

// Meta returns the meta data of the message.
func (r GeneralRsp) Meta() MsgMeta {
	return r.MsgMeta
}

// Clone returns cloned GeneralRsp with different ID
func (r GeneralRsp) Clone() Msg {
	cloneMsg := r
	cloneMsg.MsgMeta.ID = id.Generate()

	return cloneMsg
}

// GetRspTo returns the ID of the original request.
func (r GeneralRsp) GetRspTo() string {
	return r.RspTo
}

// ReqOutTaskID returns the ID of the task that is created when a request is
// sent out.
func ReqOutTaskID(reqID string) string {
	return "req_out_" + reqID
}

// ReqInTaskID returns the ID of the task that is created when a request is
// received.
func ReqInTaskID(reqID string) string {
	return "req_in_" + reqID
}
