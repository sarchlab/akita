// Package comm defines communication primitives for Akita V5.
package comm

// RemotePort identifies a port in the simulation topology.
type RemotePort string

// Msg describes the metadata contract shared by all messages in the
// communication layer. Implementations should expose their identifying fields
// via simple getters.
type Msg interface {
	ID() string
	Src() RemotePort
	Dst() RemotePort
	TrafficClass() string
	TrafficBytes() int
}

// A Rsp  is a specialized message that indicates the completion of a request.
type Rsp interface {
	Msg
	RspTo() string
}

type GeneralRsp struct {
}
