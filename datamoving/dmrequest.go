package datamoving

import (
	"github.com/sarchlab/akita/v4/sim"
)

// A DataMoveRequest asks DataMover to transfer data
type DataMoveRequest struct {
	sim.MsgMeta
	srcAddress      uint64
	dstAddress      uint64
	srcTransferSize uint64
	dstTransferSize uint64
	direction       string
	byteSize        uint64
}

func (req *DataMoveRequest) Meta() *sim.MsgMeta {
	return &req.MsgMeta
}

func (req *DataMoveRequest) Clone() sim.Msg {
	b := &DataMoveRequestBuilder{}
	b.WithSrc(req.Src)
	b.WithDst(req.Dst)
	b.WithDstAddress(req.dstAddress)
	b.WithSrcAddress(req.srcAddress)
	b.WithSrcTransferSize(req.srcTransferSize)
	b.WithDstTransferSize(req.dstTransferSize)
	b.WithDirection(req.direction)
	b.WithByteSize(req.byteSize)
	return b.Build()
}

// DataMoveRequestBuilder can build new data move requests
type DataMoveRequestBuilder struct {
	src, dst        sim.Port
	srcAddress      uint64
	dstAddress      uint64
	srcTransferSize uint64
	dstTransferSize uint64
	direction       string
	byteSize        uint64
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

func (b DataMoveRequestBuilder) WithDirection(
	inputDirection string,
) DataMoveRequestBuilder {
	b.direction = inputDirection
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
	r.srcAddress = b.srcAddress
	r.dstAddress = b.dstAddress
	r.direction = b.direction
	r.byteSize = b.byteSize
	return r
}
