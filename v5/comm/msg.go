// Package comm defines communication primitives for Akita V5.
package comm

// RemotePort identifies a port in the simulation topology.
type RemotePort string

// Msg describes the metadata contract shared by all messages in the
// communication layer. Implementations expose their identifying fields via
// simple getters.
type Msg interface {
	ID() string
	Src() RemotePort
	Dst() RemotePort
	TrafficClass() string
	TrafficBytes() int
}

// Rsp is a specialized message that indicates the completion of a request.
type Rsp interface {
	Msg
	RspTo() string
}

// GeneralRsp is a concrete response message that carries only metadata. All
// fields are exported so the struct can be serialized via standard encoders
// (e.g., JSON, gob) without additional boilerplate.
type GeneralRsp struct {
	MsgID          string     `json:"id"`
	SrcPort        RemotePort `json:"src"`
	DstPort        RemotePort `json:"dst"`
	TrafficClassID string     `json:"traffic_class,omitempty"`
	Bytes          int        `json:"traffic_bytes,omitempty"`
	RespondTo      string     `json:"rsp_to"`
	OK             bool       `json:"ok"`
}

// ID implements Msg.
func (r *GeneralRsp) ID() string {
	if r == nil {
		return ""
	}
	return r.MsgID
}

// Src implements Msg.
func (r *GeneralRsp) Src() RemotePort {
	if r == nil {
		return ""
	}
	return r.SrcPort
}

// Dst implements Msg.
func (r *GeneralRsp) Dst() RemotePort {
	if r == nil {
		return ""
	}
	return r.DstPort
}

// TrafficClass implements Msg.
func (r *GeneralRsp) TrafficClass() string {
	if r == nil {
		return ""
	}
	return r.TrafficClassID
}

// TrafficBytes implements Msg.
func (r *GeneralRsp) TrafficBytes() int {
	if r == nil {
		return 0
	}
	return r.Bytes
}

// RspTo implements Rsp.
func (r *GeneralRsp) RspTo() string {
	if r == nil {
		return ""
	}
	return r.RespondTo
}
