package datamover

import (
	"github.com/sarchlab/akita/v5/messaging"
)

// DataMovePort is the port name that either serves as a source or destination.
// It can be either inside or outside.
type DataMovePort string

const (
	InsidePort  DataMovePort = "inside"
	OutsidePort DataMovePort = "outside"
)

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
