// Package datamoverprotocol defines the data move protocol: the message types
// a requester uses to ask a data mover to copy a byte range, and the protocol
// roles ports bind to.
package datamoverprotocol

import "github.com/sarchlab/akita/v5/sim/messaging"

// Protocol is the data move protocol: a requester asks the data mover to copy
// a byte range between its inside and outside sides, and the data mover
// responds on completion. Defining the protocol registers every message type
// it carries with the checkpoint codec.
var (
	Protocol = messaging.DefineProtocol(
		messaging.RoleDef{Name: "requester",
			Sends: []any{Req{}}},
		messaging.RoleDef{Name: "responder",
			Sends: []any{Rsp{}}},
	)
	Requester = Protocol.Role("requester")
	Responder = Protocol.Role("responder")
)

// Side identifies the source or destination side of a data move.
// It can be either inside or outside.
type Side string

const (
	SideInside  Side = "inside"
	SideOutside Side = "outside"
)

// Rsp is sent when a data move operation completes.
type Rsp struct {
}

// Req is a data move request.
type Req struct {
	SrcAddress uint64
	DstAddress uint64
	ByteSize   uint64
	SrcSide    Side
	DstSide    Side
}
