package datamoving

import (
	"github.com/sarchlab/akita/v4/sim"
)

// A DataMoveRequest asks DataMover to transfer data
type DataMoveRequest struct {
	sim.MsgMeta
	srcAddress   uint64
	dstAddress   uint64
	srcBuffer    []byte
	dstBuffer    []byte
	srcDirection string
	dstDirection string
	byteSize     uint64
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
	b.WithSrcBuffer(req.srcBuffer)
	b.WithDstBuffer(req.dstBuffer)
	b.WithSrcDirection(req.srcDirection)
	b.WithDstDirection(req.dstDirection)
	b.WithByteSize(req.byteSize)
	return b.Build()
}

// DataMoveRequestBuilder can build new data move requests
type DataMoveRequestBuilder struct {
	src, dst     sim.Port
	srcAddress   uint64
	dstAddress   uint64
	srcBuffer    []byte
	dstBuffer    []byte
	srcDirection string
	dstDirection string
	byteSize     uint64
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

func (b DataMoveRequestBuilder) WithSrcBuffer(
	inputSrcBuffer []byte,
) DataMoveRequestBuilder {
	b.srcBuffer = inputSrcBuffer
	return b
}

func (b DataMoveRequestBuilder) WithDstBuffer(
	inputDstBuffer []byte,
) DataMoveRequestBuilder {
	b.dstBuffer = inputDstBuffer
	return b
}

func (b DataMoveRequestBuilder) WithSrcDirection(
	inputSrcDirection string,
) DataMoveRequestBuilder {
	b.srcDirection = inputSrcDirection
	return b
}

func (b DataMoveRequestBuilder) WithDstDirection(
	inputDstDirection string,
) DataMoveRequestBuilder {
	b.dstDirection = inputDstDirection
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
	r.srcBuffer = b.srcBuffer
	r.dstBuffer = b.dstBuffer
	r.srcDirection = b.srcDirection
	r.dstDirection = b.dstDirection
	r.byteSize = b.byteSize
	return r
}
