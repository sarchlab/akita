package modeling

import (
	"github.com/sarchlab/akita/v4/sim/id"
	"github.com/sarchlab/akita/v4/sim/serialization"
)

// A Msg is a piece of information that is transferred between components.
type Msg interface {
	serialization.Serializable

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

	OriginalReq Msg
}

// Meta returns the meta data of the message.
func (r GeneralRsp) Meta() MsgMeta {
	return r.MsgMeta
}

// Serialize serializes the message.
func (r GeneralRsp) Serialize() (map[string]interface{}, error) {
	return map[string]interface{}{
		"id":            r.ID,
		"src":           r.Src,
		"dst":           r.Dst,
		"traffic_class": r.TrafficClass,
		"traffic_bytes": r.TrafficBytes,
		"original_req":  r.OriginalReq.ID(),
	}, nil
}

// Deserialize deserializes the message.
func (r GeneralRsp) Deserialize(data map[string]interface{}) error {
	r.ID = data["id"].(string)
	r.Src = data["src"].(RemotePort)
	r.Dst = data["dst"].(RemotePort)
	r.TrafficClass = data["traffic_class"].(int)
	r.TrafficBytes = data["traffic_bytes"].(int)
	r.OriginalReq = data["original_req"].(Msg)

	return nil
}

// Clone returns cloned GeneralRsp with different ID
func (r GeneralRsp) Clone() Msg {
	cloneMsg := r
	cloneMsg.ID = id.Generate()

	return cloneMsg
}

// GetRspTo returns the ID of the original request.
func (r GeneralRsp) GetRspTo() string {
	return r.OriginalReq.Meta().ID
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
