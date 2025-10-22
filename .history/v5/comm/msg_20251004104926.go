// Package comm defines communication primitives for Akita V5.
package comm

// RemotePort identifies a port in the simulation topology.
type RemotePort string

// Msg describes the metadata contract shared by all messages in the
// communication layer. Implementations expose their identifying fields via
// simple getters.
type Msg interface {
	MessageID() string
	SrcPort() RemotePort
	DstPort() RemotePort
	MessageClass() string
	MessageBytes() int
}

// Rsp is a specialized message that indicates the completion of a request.
type Rsp interface {
	Msg
	RespondsTo() string
}

// GeneralRsp is a concrete response message that carries only metadata. All
// fields are exported so the struct can be serialized via standard encoders
// (e.g., JSON, gob) without additional boilerplate.
type GeneralRsp struct {
	ID           string     `json:"id"`
	Src          RemotePort `json:"src"`
	Dst          RemotePort `json:"dst"`
	TrafficClass string     `json:"traffic_class,omitempty"`
	TrafficBytes int        `json:"traffic_bytes,omitempty"`
	RspTo        string     `json:"rsp_to"`
	OK           bool       `json:"ok"`
}

type generalRspView GeneralRsp

// MessageID implements Msg.
func (r *GeneralRsp) MessageID() string {
	if r == nil {
		return ""
	}
	return (*generalRspView)(r).ID
}

// SrcPort implements Msg.
func (r *GeneralRsp) SrcPort() RemotePort {
	if r == nil {
		return ""
	}
	return (*generalRspView)(r).Src
}

// DstPort implements Msg.
func (r *GeneralRsp) DstPort() RemotePort {
	if r == nil {
		return ""
	}
	return (*generalRspView)(r).Dst
}

// MessageClass implements Msg.
func (r *GeneralRsp) MessageClass() string {
	if r == nil {
		return ""
	}
	return (*generalRspView)(r).TrafficClass
}

// MessageBytes implements Msg.
func (r *GeneralRsp) MessageBytes() int {
	if r == nil {
		return 0
	}
	return (*generalRspView)(r).TrafficBytes
}

// RespondsTo implements Rsp.
func (r *GeneralRsp) RespondsTo() string {
	if r == nil {
		return ""
	}
	return (*generalRspView)(r).RspTo
}
