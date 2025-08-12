package datamover

import (
	"reflect"

	"github.com/sarchlab/akita/v4/sim"
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
	sim.MsgMeta

	SrcAddress uint64
	DstAddress uint64
	ByteSize   uint64
	SrcSide    DateMovePort
	DstSide    DateMovePort
}

// Meta returns the metadata of the message
func (req *DataMoveRequest) Meta() *sim.MsgMeta {
	return &req.MsgMeta
}

// Clone creates a deep copy of the DataMoveRequest with a new ID
func (req *DataMoveRequest) Clone() sim.Msg {
	b := MakeDataMoveRequestBuilder().
		WithSrc(req.Src).
		WithDst(req.Dst).
		WithDstAddress(req.DstAddress).
		WithSrcAddress(req.SrcAddress).
		WithSrcSide(req.SrcSide).
		WithDstSide(req.DstSide).
		WithByteSize(req.ByteSize)

	return b.Build()
}

// GenerateRsp creates a response message for the request.
func (req *DataMoveRequest) GenerateRsp() sim.Msg {
	rsp := sim.GeneralRspBuilder{}.
		WithSrc(req.Dst).
		WithDst(req.Src).
		WithOriginalReq(req).
		Build()

	return rsp
}

// DataMoveRequestBuilder can build new data move requests
type DataMoveRequestBuilder struct {
	src, dst   sim.RemotePort
	srcAddress uint64
	dstAddress uint64
	byteSize   uint64
	srcSide    DateMovePort
	dstSide    DateMovePort
}

// MakeDataMoveRequestBuilder creates a new DataMoveRequestBuilder
func MakeDataMoveRequestBuilder() DataMoveRequestBuilder {
	return DataMoveRequestBuilder{}
}

// WithSrc sets the source port of the message.
func (b DataMoveRequestBuilder) WithSrc(
	inputSrc sim.RemotePort,
) DataMoveRequestBuilder {
	b.src = inputSrc
	return b
}

// WithDst sets the destination port of the message. It should be the CtrlPort
// of the DataMover.
func (b DataMoveRequestBuilder) WithDst(
	inputDst sim.RemotePort,
) DataMoveRequestBuilder {
	b.dst = inputDst
	return b
}

// WithSrcAddress sets the source address of the data to be moved.
func (b DataMoveRequestBuilder) WithSrcAddress(
	inputSrcAddress uint64,
) DataMoveRequestBuilder {
	b.srcAddress = inputSrcAddress
	return b
}

// WithDstAddress sets the destination address of the data to be moved.
func (b DataMoveRequestBuilder) WithDstAddress(
	inputDstAddress uint64,
) DataMoveRequestBuilder {
	b.dstAddress = inputDstAddress
	return b
}

// WithDstSide sets the destination side of the data to be moved.
func (b DataMoveRequestBuilder) WithDstSide(
	side DateMovePort,
) DataMoveRequestBuilder {
	b.dstSide = side
	return b
}

// WithSrcSide sets the source side of the data to be moved.
func (b DataMoveRequestBuilder) WithSrcSide(
	side DateMovePort,
) DataMoveRequestBuilder {
	b.srcSide = side
	return b
}

// WithByteSize sets the byte size of the data to be moved.
func (b DataMoveRequestBuilder) WithByteSize(
	inputByteSize uint64,
) DataMoveRequestBuilder {
	b.byteSize = inputByteSize
	return b
}

// Build creates a new DataMoveRequest.
func (b DataMoveRequestBuilder) Build() *DataMoveRequest {
	r := &DataMoveRequest{}
	r.ID = sim.GetIDGenerator().Generate()
	r.Src = b.src
	r.Dst = b.dst
	r.SrcAddress = b.srcAddress
	r.DstAddress = b.dstAddress
	r.ByteSize = b.byteSize
	r.SrcSide = b.srcSide
	r.DstSide = b.dstSide
	r.TrafficClass = reflect.TypeOf(DataMoveRequest{}).String()

	return r
}
