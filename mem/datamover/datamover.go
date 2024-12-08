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

type buffer struct {
	initAddr    uint64
	granularity uint64
	data        [][]byte
}

func (b *buffer) addData(addr uint64, data []byte) {
	addressMustBeAligned(addr, b.granularity)

	offset := (addr - b.initAddr) / b.granularity
	for i := uint64(len(b.data)); i <= offset; i++ {
		b.data = append(b.data, nil)
	}

	b.data[offset] = data
}

func (b *buffer) extractData(addr, size uint64) (data []byte, ok bool) {
	data = make([]byte, size)

	sizeLeft := size
	offset := (addr - b.initAddr) / b.granularity

	for i := offset; i < uint64(len(b.data)); i++ {
		if b.data[i] == nil {
			return nil, false
		}

		copySize := min(sizeLeft, uint64(len(b.data[i])))
		copy(data[size-sizeLeft:], b.data[i][:copySize])
		sizeLeft -= copySize

		if sizeLeft == 0 {
			return data, true
		}
	}

	return nil, false
}

func (b *buffer) moveInitAddrForwardTo(newStart uint64) {
	alignedNewStart := (newStart / b.granularity) * b.granularity

	if alignedNewStart <= b.initAddr {
		return
	}

	discardChunks := (alignedNewStart - b.initAddr) / b.granularity
	if discardChunks > uint64(len(b.data)) {
		b.data = b.data[:0]
	} else {
		b.data = b.data[discardChunks:]
	}

	b.initAddr = alignedNewStart
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

	toSrc           []sim.Msg
	toDst           []sim.Msg
	toCP            []sim.Msg
	pendingRequests []sim.Msg
	bufferSize      uint64
	buffer          buffer

	srcPort            sim.Port
	dstPort            sim.Port
	srcPortMapper      mem.AddressToPortMapper
	dstPortMapper      mem.AddressToPortMapper
	srcByteGranularity uint64
	dstByteGranularity uint64
	currentTransaction *dataMoverTransaction
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

	rqC := &dataMoverTransaction{
		req:           moveReq,
		nextReadAddr:  moveReq.SrcAddress,
		nextWriteAddr: moveReq.DstAddress,
	}
	c.currentTransaction = rqC
	c.buffer = buffer{
		initAddr:    moveReq.DstAddress,
		granularity: c.dstByteGranularity,
	}

	c.setSrcSide(moveReq)
	c.setDstSide(moveReq)

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

	bufEndAddr := trans.nextWriteAddr + c.bufferSize
	if addr >= bufEndAddr {
		return false
	}

	transEndAddr := trans.req.SrcAddress + trans.req.ByteSize
	if addr > transEndAddr {
		return false
	}

	req := mem.ReadReqBuilder{}.
		WithAddress(addr).
		WithSrc(c.srcPort).
		WithDst(c.dstPortMapper.Find(addr)).
		WithByteSize(c.srcByteGranularity).
		WithPID(0).
		Build()

	err := c.srcPort.Send(req)
	if err != nil {
		return false
	}

	trans.nextReadAddr += addr + c.srcByteGranularity
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

	c.buffer.addData(originalReq.Address, readRsp.Data)

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

	data, ok := c.buffer.extractData(
		c.currentTransaction.nextWriteAddr, c.dstByteGranularity)

	if !ok {
		return false
	}

	req := mem.WriteReqBuilder{}.
		WithAddress(c.currentTransaction.nextWriteAddr).
		WithData(data).
		WithSrc(c.dstPort).
		WithDst(c.dstPortMapper.Find(c.currentTransaction.nextWriteAddr)).
		WithPID(0).
		Build()

	err := c.dstPort.Send(req)
	if err != nil {
		return false
	}

	c.currentTransaction.nextWriteAddr += c.dstByteGranularity
	c.currentTransaction.pendingWrite[req.ID] = req
	c.buffer.moveInitAddrForwardTo(c.currentTransaction.nextWriteAddr)

	tracing.TraceReqInitiate(req, c,
		tracing.MsgIDAtReceiver(c.currentTransaction.req, c))

	return true
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
	c.buffer = buffer{}

	tracing.TraceReqComplete(rsp, c)

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
