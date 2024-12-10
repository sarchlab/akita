package datamover

import (
	"log"
	"reflect"

	"github.com/sarchlab/akita/v4/mem/mem"
	"github.com/sarchlab/akita/v4/sim"
	"github.com/sarchlab/akita/v4/tracing"
)

// A dataMoverTransaction contains a data moving request from a single
// source/destination with Read/Write requests correspond to it.
type dataMoverTransaction struct {
	req           *DataMoveRequest
	nextReadAddr  uint64
	nextWriteAddr uint64
	pendingRead   map[string]*mem.ReadReq
	pendingWrite  map[string]*mem.WriteReq
}

func newDataMoverTransaction(req *DataMoveRequest) *dataMoverTransaction {
	return &dataMoverTransaction{
		req:           req,
		nextReadAddr:  req.SrcAddress,
		nextWriteAddr: req.DstAddress,
		pendingRead:   make(map[string]*mem.ReadReq),
		pendingWrite:  make(map[string]*mem.WriteReq),
	}
}

func alignAddress(addr, granularity uint64) uint64 {
	return addr / granularity * granularity
}

func addressMustBeAligned(addr, granularity uint64) {
	if addr%granularity != 0 {
		log.Panicf("address %d must be aligned to %d", addr, granularity)
	}
}

// Comp helps moving data from designated source and destination
// following the given move direction
type Comp struct {
	*sim.TickingComponent

	ctrlPort    sim.Port
	insidePort  sim.Port
	outsidePort sim.Port

	insidePortMapper       mem.AddressToPortMapper
	outsidePortMapper      mem.AddressToPortMapper
	insideByteGranularity  uint64
	outsideByteGranularity uint64

	srcPort            sim.Port
	dstPort            sim.Port
	srcPortMapper      mem.AddressToPortMapper
	dstPortMapper      mem.AddressToPortMapper
	srcByteGranularity uint64
	dstByteGranularity uint64
	currentTransaction *dataMoverTransaction
	bufferSize         uint64
	buffer             *buffer
}

// Tick ticks
func (c *Comp) Tick() bool {
	madeProgress := false

	madeProgress = c.finishTransaction() || madeProgress
	madeProgress = c.processWriteDoneFromDst() || madeProgress
	madeProgress = c.writeToDst() || madeProgress
	madeProgress = c.processDataReadyFromSrc() || madeProgress
	madeProgress = c.readFromSrc() || madeProgress
	madeProgress = c.parseFromCP() || madeProgress

	return madeProgress
}

// parseFromCP retrieves Msg from ctrlPort
func (c *Comp) parseFromCP() bool {
	req := c.ctrlPort.RetrieveIncoming()
	if req == nil {
		return false
	}

	if c.currentTransaction != nil {
		return false
	}

	moveReq, ok := req.(*DataMoveRequest)
	if !ok {
		log.Panicf("can't process request of type %s", reflect.TypeOf(req))
	}

	trans := newDataMoverTransaction(moveReq)
	c.currentTransaction = trans

	c.setSrcSide(moveReq)
	c.setDstSide(moveReq)

	c.buffer = &buffer{
		granularity: c.srcByteGranularity,
	}

	tracing.TraceReqReceive(req, c)

	return true
}

// readFromSrc reads data from source
func (c *Comp) readFromSrc() bool {
	if c.currentTransaction == nil {
		return false
	}

	trans := c.currentTransaction
	addr := alignAddress(trans.nextReadAddr, c.srcByteGranularity)

	bufEndAddr := c.buffer.offset + c.bufferSize
	if addr >= bufEndAddr {
		return false
	}

	transEndAddr := trans.req.SrcAddress + trans.req.ByteSize
	if addr > transEndAddr {
		return false
	}

	req := mem.ReadReqBuilder{}.
		WithAddress(addr).
		WithSrc(c.srcPort.AsRemote()).
		WithDst(c.srcPortMapper.Find(addr)).
		WithByteSize(c.srcByteGranularity).
		WithPID(0).
		Build()

	err := c.srcPort.Send(req)
	if err != nil {
		return false
	}

	trans.nextReadAddr += c.srcByteGranularity
	trans.pendingRead[req.ID] = req

	tracing.TraceReqInitiate(req, c, tracing.MsgIDAtReceiver(trans.req, c))

	return true
}

// processDataReadyFromSrc processes data ready from source
func (c *Comp) processDataReadyFromSrc() bool {
	if c.currentTransaction == nil {
		return false
	}

	rsp := c.srcPort.PeekIncoming()
	if rsp == nil {
		return false
	}

	readRsp, ok := rsp.(*mem.DataReadyRsp)
	if !ok {
		// it can be write done rsp if src and dst is the same side. So ignore.
		return false
	}

	originalReq, ok := c.currentTransaction.pendingRead[readRsp.RespondTo]
	if !ok {
		log.Panicf("can't find original request for response %s",
			readRsp.RespondTo)
	}

	offset := originalReq.Address - c.currentTransaction.req.SrcAddress
	c.buffer.addData(offset, readRsp.Data)

	delete(c.currentTransaction.pendingRead, readRsp.RespondTo)
	c.srcPort.RetrieveIncoming()
	tracing.TraceReqFinalize(originalReq, c)

	return true
}

// writeToDst sends data to destination
func (c *Comp) writeToDst() bool {
	if c.currentTransaction == nil {
		return false
	}

	trans := c.currentTransaction
	offset := trans.nextWriteAddr - trans.req.DstAddress
	data, ok := c.buffer.extractData(offset, c.dstByteGranularity)

	if !ok {
		return false
	}

	req := mem.WriteReqBuilder{}.
		WithAddress(c.currentTransaction.nextWriteAddr).
		WithData(data).
		WithSrc(c.dstPort.AsRemote()).
		WithDst(c.dstPortMapper.Find(c.currentTransaction.nextWriteAddr)).
		WithPID(0).
		Build()

	err := c.dstPort.Send(req)
	if err != nil {
		return false
	}

	c.currentTransaction.nextWriteAddr += c.dstByteGranularity
	c.currentTransaction.pendingWrite[req.ID] = req
	c.buffer.moveOffsetForwardTo(trans.nextWriteAddr - trans.req.DstAddress)

	tracing.TraceReqInitiate(req, c,
		tracing.MsgIDAtReceiver(c.currentTransaction.req, c))

	return true
}

// processWriteDoneFromDst processes write done from destination
func (c *Comp) processWriteDoneFromDst() bool {
	if c.currentTransaction == nil {
		return false
	}

	rsp := c.dstPort.PeekIncoming()
	if rsp == nil {
		return false
	}

	writeRsp, ok := rsp.(*mem.WriteDoneRsp)
	if !ok {
		return false
	}

	originalReq, ok := c.currentTransaction.pendingWrite[writeRsp.RespondTo]
	if !ok {
		log.Panicf("can't find original request for response %s",
			writeRsp.RespondTo)
	}

	delete(c.currentTransaction.pendingWrite, writeRsp.RespondTo)
	c.dstPort.RetrieveIncoming()

	tracing.TraceReqFinalize(originalReq, c)

	return false
}

// finishTransaction finishes the current transaction
func (c *Comp) finishTransaction() bool {
	if c.currentTransaction == nil {
		return false
	}

	trans := c.currentTransaction

	if trans.nextWriteAddr < trans.req.DstAddress+trans.req.ByteSize {
		return false
	}

	rsp := trans.req.GenerateRsp()

	err := c.ctrlPort.Send(rsp)
	if err != nil {
		return false
	}

	c.currentTransaction = nil
	c.buffer = &buffer{
		offset:      alignAddress(trans.req.SrcAddress, c.srcByteGranularity),
		granularity: c.srcByteGranularity,
	}

	tracing.TraceReqComplete(rsp, c)

	return true
}

func (c *Comp) setSrcSide(moveReq *DataMoveRequest) {
	switch moveReq.SrcSide {
	case "inside":
		c.srcPort = c.insidePort
		c.srcPortMapper = c.insidePortMapper
		c.srcByteGranularity = c.insideByteGranularity
	case "outside":
		c.srcPort = c.outsidePort
		c.srcPortMapper = c.outsidePortMapper
		c.srcByteGranularity = c.outsideByteGranularity
	default:
		log.Panicf("can't process source port of type %s", moveReq.SrcSide)
	}

	addressMustBeAligned(moveReq.SrcAddress, c.srcByteGranularity)
}

func (c *Comp) setDstSide(moveReq *DataMoveRequest) {
	switch moveReq.DstSide {
	case "inside":
		c.dstPort = c.insidePort
		c.dstPortMapper = c.insidePortMapper
		c.dstByteGranularity = c.insideByteGranularity
	case "outside":
		c.dstPort = c.outsidePort
		c.dstPortMapper = c.outsidePortMapper
		c.dstByteGranularity = c.outsideByteGranularity
	default:
		log.Panicf("can't process destination port of type %s", moveReq.DstSide)
	}

	addressMustBeAligned(moveReq.DstAddress, c.dstByteGranularity)
}
