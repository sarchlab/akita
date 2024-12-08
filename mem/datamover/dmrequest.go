package datamover

import (
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
	SrcAddress      uint64
	DstAddress      uint64
	SrcTransferSize uint64
	DstTransferSize uint64
	ByteSize        uint64
	SrcPort         DateMovePort
	DstPort         DateMovePort
}

func (req *DataMoveRequest) Meta() *sim.MsgMeta {
	return &req.MsgMeta
}

func (req *DataMoveRequest) Clone() sim.Msg {
	b := &DataMoveRequestBuilder{}
	b.WithSrc(req.Src)
	b.WithDst(req.Dst)
	b.WithDstAddress(req.DstAddress)
	b.WithSrcAddress(req.SrcAddress)
	b.WithSrcTransferSize(req.SrcTransferSize)
	b.WithDstTransferSize(req.DstTransferSize)
	b.WithSrcPort(req.SrcPort)
	b.WithDstPort(req.DstPort)
	b.WithByteSize(req.ByteSize)
	return b.Build()
}

// DataMoveRequestBuilder can build new data move requests
type DataMoveRequestBuilder struct {
	src, dst        sim.Port
	srcAddress      uint64
	dstAddress      uint64
	srcTransferSize uint64
	dstTransferSize uint64
	byteSize        uint64
	srcPort         DateMovePort
	dstPort         DateMovePort
}

// MakeDataMoveRequestBuilder creates a new DataMoveRequestBuilder
func MakeDataMoveRequestBuilder() DataMoveRequestBuilder {
	return DataMoveRequestBuilder{}
}

func (b DataMoveRequestBuilder) WithSrc(
	inputSrc sim.Port,
) DataMoveRequestBuilder {
	b.src = inputSrc
	return b
}

func (b DataMoveRequestBuilder) WithDst(
	inputDst sim.Port,
) DataMoveRequestBuilder {
	b.dst = inputDst
	return b
}

func (b DataMoveRequestBuilder) WithSrcAddress(
	inputSrcAddress uint64,
) DataMoveRequestBuilder {
	b.srcAddress = inputSrcAddress
	return b
}

func (b DataMoveRequestBuilder) WithDstAddress(
	inputDstAddress uint64,
) DataMoveRequestBuilder {
	b.dstAddress = inputDstAddress
	return b
}

func (b DataMoveRequestBuilder) WithSrcTransferSize(
	inputSrcTransferSize uint64,
) DataMoveRequestBuilder {
	b.srcTransferSize = inputSrcTransferSize
	return b
}

func (b DataMoveRequestBuilder) WithDstTransferSize(
	inputDstTransferSize uint64,
) DataMoveRequestBuilder {
	b.dstTransferSize = inputDstTransferSize
	return b
}

func (b DataMoveRequestBuilder) WithDstPort(
	inputDstPort DateMovePort,
) DataMoveRequestBuilder {
	b.dstPort = inputDstPort
	return b
}

func (b DataMoveRequestBuilder) WithSrcPort(
	inputSrcPort DateMovePort,
) DataMoveRequestBuilder {
	b.srcPort = inputSrcPort
	return b
}

func (b DataMoveRequestBuilder) WithByteSize(
	inputByteSize uint64,
) DataMoveRequestBuilder {
	b.byteSize = inputByteSize
	return b
}

func (b DataMoveRequestBuilder) Build() *DataMoveRequest {
	r := &DataMoveRequest{}
	r.ID = sim.GetIDGenerator().Generate()
	r.Src = b.src
	r.Dst = b.dst
	r.SrcAddress = b.srcAddress
	r.DstAddress = b.dstAddress
	r.SrcTransferSize = b.srcTransferSize
	r.DstTransferSize = b.dstTransferSize
	r.ByteSize = b.byteSize
	r.SrcPort = b.srcPort
	r.DstPort = b.dstPort
	return r
}
