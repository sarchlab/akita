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

// DataMoveRequest is a data move request.
type DataMoveRequest struct {
	sim.MsgMeta
	SrcAddress uint64
	DstAddress uint64
	ByteSize   uint64
	SrcSide    DateMovePort
	DstSide    DateMovePort
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
	r := &DataMoveRequest{
		SrcAddress: b.srcAddress,
		DstAddress: b.dstAddress,
		ByteSize:   b.byteSize,
		SrcSide:    b.srcSide,
		DstSide:    b.dstSide,
	}
	r.ID = sim.GetIDGenerator().Generate()
	r.Src = b.src
	r.Dst = b.dst
	r.TrafficClass = "datamover.DataMoveRequest"
	return r
}
