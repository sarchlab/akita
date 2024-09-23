package datamoving

import (
	"github.com/sarchlab/akita/v4/sim"
)

// A DataMoveRequest asks DataMover to transfer data
type DataMoveRequest struct {
	sim.MsgMeta
	srcAddress   uint64
	dstAddress   uint64
	srcDirection string
	dstDirection string
	byteSize     uint64
}

func (req *DataMoveRequest) Meta() *sim.MsgMeta {
	return &req.MsgMeta
}

func (req *DataMoveRequest) Clone() sim.Msg {
	return req
}

// DataMoveRequestBuilder can build new data move requests
type DataMoveRequestBuilder struct {
	src, dst     sim.Port
	srcAddress   uint64
	dstAddress   uint64
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
	r.srcDirection = b.srcDirection
	r.dstDirection = b.dstDirection
	r.byteSize = b.byteSize
	return r
}
