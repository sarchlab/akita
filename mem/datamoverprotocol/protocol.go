// Package datamoverprotocol defines the data move protocol: the message types
// a requester uses to ask a data mover to copy a byte range, and the protocol
// roles ports bind to.
package datamoverprotocol

import (
	"github.com/sarchlab/akita/v5/messaging"
)

// Protocol is the data move protocol: a requester asks the data mover to copy
// a byte range between its inside and outside sides, and the data mover
// responds on completion. Defining the protocol registers every message type
// it carries with the checkpoint codec.
var (
	Protocol = messaging.DefineProtocol("datamover",
		messaging.RoleDef{Name: "requester",
			Sends: []messaging.Msg{DataMoveRequest{}}},
		messaging.RoleDef{Name: "responder",
			Sends: []messaging.Msg{DataMoveResponse{}}},
	)
	Requester = Protocol.Role("requester")
	Responder = Protocol.Role("responder")
)

// DataMovePort is the port name that either serves as a source or destination.
// It can be either inside or outside.
type DataMovePort string

// DataMoveResponse is sent when a data move operation completes.
type DataMoveResponse struct {
	messaging.MsgMeta
}

// DataMoveRequest is a data move request.
type DataMoveRequest struct {
	messaging.MsgMeta
	SrcAddress uint64
	DstAddress uint64
	ByteSize   uint64
	SrcSide    DataMovePort
	DstSide    DataMovePort
}
