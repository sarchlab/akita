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

func (b *DataMoveRequestBuilder) WithSrc(
	inputSrc sim.Port,
) {
	b.src = inputSrc
}

func (b *DataMoveRequestBuilder) WithDst(
	inputDst sim.Port,
) {
	b.dst = inputDst
}

func (b *DataMoveRequestBuilder) WithSrcAddress(
	inputSrcAddress uint64,
) {
	b.srcAddress = inputSrcAddress
}

func (b *DataMoveRequestBuilder) WithDstAddress(
	inputDstAddress uint64,
) {
	b.dstAddress = inputDstAddress
}

func (b *DataMoveRequestBuilder) WithSrcTransferSize(
	inputSrcTransferSize uint64,
) {
	b.srcTransferSize = inputSrcTransferSize
}

func (b *DataMoveRequestBuilder) WithDstTransferSize(
	inputDstTransferSize uint64,
) {
	b.dstTransferSize = inputDstTransferSize
}

func (b *DataMoveRequestBuilder) WithDirection(
	inputDirection string,
) {
	b.direction = inputDirection
}

func (b *DataMoveRequestBuilder) WithByteSize(
	inputByteSize uint64,
) {
	b.byteSize = inputByteSize
}

func (b *DataMoveRequestBuilder) Build() *DataMoveRequest {
	r := &DataMoveRequest{}
	r.ID = sim.GetIDGenerator().Generate()
	r.Src = b.src
	r.Dst = b.dst
	r.srcAddress = b.srcAddress
	r.dstAddress = b.dstAddress
	r.srcTransferSize = b.srcTransferSize
	r.dstTransferSize = b.dstTransferSize
	r.direction = b.direction
	r.byteSize = b.byteSize
	return r
}
