package datamover

import (
	"github.com/sarchlab/akita/v4/sim/id"
	"github.com/sarchlab/akita/v4/sim/modeling"
)

// DateMovePort is the port name that either serves as a source or destination.
// It can be either inside or outside.
type DateMovePort string

const (
	InsidePort  DateMovePort = "inside"
	OutsidePort DateMovePort = "outside"
)

// A DataMoveRequest asks DataMover to transfer data
type DataMoveRequest struct {
	modeling.MsgMeta
	SrcAddress uint64
	DstAddress uint64
	ByteSize   uint64
	SrcSide    DateMovePort
	DstSide    DateMovePort
}

// Meta returns the metadata of the message
func (req DataMoveRequest) Meta() modeling.MsgMeta {
	return req.MsgMeta
}

// Clone creates a deep copy of the DataMoveRequest with a new ID
func (req DataMoveRequest) Clone() modeling.Msg {
	cloneMsg := req
	cloneMsg.ID = id.Generate()

	return cloneMsg
}

// GenerateRsp creates a response message for the request.
func (req DataMoveRequest) GenerateRsp() modeling.Msg {
	rsp := modeling.GeneralRsp{
		MsgMeta: modeling.MsgMeta{
			Src: req.Dst,
			Dst: req.Src,
			ID:  id.Generate(),
		},
		RspTo: req.ID,
	}

	return rsp
}
