package datamover

import (
	"github.com/sarchlab/akita/v5/sim"
)

// DateMovePort is the port name that either serves as a source or destination.
// It can be either inside or outside.
type DateMovePort string

const (
	InsidePort  DateMovePort = "inside"
	OutsidePort DateMovePort = "outside"
)

// DataMoveResponse is sent when a data move operation completes.
type DataMoveResponse struct {
	sim.MsgMeta
}

// DataMoveRequest is a data move request.
type DataMoveRequest struct {
	sim.MsgMeta
	SrcAddress uint64
	DstAddress uint64
	ByteSize   uint64
	SrcSide    DateMovePort
	DstSide    DateMovePort
}
