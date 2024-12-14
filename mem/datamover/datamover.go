package datamover

import (
	"log"
	"reflect"

	"github.com/sarchlab/akita/v4/mem"
	"github.com/sarchlab/akita/v4/sim/hooking"
	"github.com/sarchlab/akita/v4/sim/id"
	"github.com/sarchlab/akita/v4/sim/modeling"
)

// A dataMoverTransaction contains a data moving request from a single
// source/destination with Read/Write requests correspond to it.
type dataMoverTransaction struct {
	req           DataMoveRequest
	nextReadAddr  uint64
	nextWriteAddr uint64
	pendingRead   map[string]mem.ReadReq
	pendingWrite  map[string]mem.WriteReq
}

func newDataMoverTransaction(req DataMoveRequest) *dataMoverTransaction {
	return &dataMoverTransaction{
		req:           req,
		nextReadAddr:  req.SrcAddress,
		nextWriteAddr: req.DstAddress,
		pendingRead:   make(map[string]mem.ReadReq),
		pendingWrite:  make(map[string]mem.WriteReq),
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
	*modeling.TickingComponent

	ctrlPort    modeling.Port
	insidePort  modeling.Port
	outsidePort modeling.Port

	insidePortMapper       mem.AddressToPortMapper
	outsidePortMapper      mem.AddressToPortMapper
	insideByteGranularity  uint64
	outsideByteGranularity uint64

	srcPort            modeling.Port
	dstPort            modeling.Port
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

	moveReq, ok := req.(DataMoveRequest)
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

	c.traceDataMoveStart(moveReq)

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

	req := mem.ReadReq{
		MsgMeta: modeling.MsgMeta{
			Src: c.srcPort.AsRemote(),
			Dst: c.srcPortMapper.Find(addr),
			ID:  id.Generate(),
		},
		Address:            addr,
		AccessByteSize:     c.srcByteGranularity,
		CanWaitForCoalesce: false,
	}

	err := c.srcPort.Send(req)
	if err != nil {
		return false
	}

	trans.nextReadAddr += c.srcByteGranularity
	trans.pendingRead[req.ID] = req

	c.traceReadWriteStart(req)

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

	readRsp, ok := rsp.(mem.DataReadyRsp)
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

	c.traceReadWriteEnd(originalReq)

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

	req := mem.WriteReq{
		MsgMeta: modeling.MsgMeta{
			Src: c.dstPort.AsRemote(),
			Dst: c.dstPortMapper.Find(c.currentTransaction.nextWriteAddr),
			ID:  id.Generate(),
		},
		Address:            c.currentTransaction.nextWriteAddr,
		Data:               data,
		CanWaitForCoalesce: false,
	}

	err := c.dstPort.Send(req)
	if err != nil {
		return false
	}

	c.currentTransaction.nextWriteAddr += c.dstByteGranularity
	c.currentTransaction.pendingWrite[req.ID] = req
	c.buffer.moveOffsetForwardTo(trans.nextWriteAddr - trans.req.DstAddress)

	c.traceReadWriteStart(req)

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

	writeRsp, ok := rsp.(mem.WriteDoneRsp)
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

	c.traceReadWriteEnd(originalReq)

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

	c.traceDataMoveEnd(trans.req)

	return true
}

func (c *Comp) setSrcSide(moveReq DataMoveRequest) {
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

func (c *Comp) setDstSide(moveReq DataMoveRequest) {
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

func (c *Comp) traceDataMoveStart(req DataMoveRequest) {
	ctx := hooking.HookCtx{
		Domain: c,
		Pos:    hooking.HookPosTaskStart,
		Item: hooking.TaskStart{
			ID:       modeling.ReqInTaskID(req.Meta().ID),
			ParentID: modeling.ReqOutTaskID(req.Meta().ID),
			Kind:     "req_in",
			What:     reflect.TypeOf(req).String(),
		},
	}

	c.InvokeHook(ctx)
}

func (c *Comp) traceDataMoveEnd(req DataMoveRequest) {
	ctx := hooking.HookCtx{
		Domain: c,
		Pos:    hooking.HookPosTaskEnd,
		Item:   hooking.TaskEnd{ID: modeling.ReqInTaskID(req.Meta().ID)},
	}

	c.InvokeHook(ctx)
}

func (c *Comp) traceReadWriteStart(req mem.AccessReq) {
	ctx := hooking.HookCtx{
		Domain: c,
		Pos:    hooking.HookPosTaskStart,
		Item: hooking.TaskStart{
			ID:       modeling.ReqOutTaskID(req.Meta().ID),
			ParentID: modeling.ReqInTaskID(c.currentTransaction.req.Meta().ID),
			Kind:     "req_out",
			What:     reflect.TypeOf(req).String(),
		},
	}

	// switch req := req.(type) {
	// case mem.ReadReq:
	// 	fmt.Printf("%.10f, %s, start read, 0x%016x\n",
	// 		c.Now(), c.Name(), req.Address)
	// case mem.WriteReq:
	// 	fmt.Printf("%.10f, %s, start write, 0x%016x, %v\n",
	// 		c.Now(), c.Name(), req.Address, req.Data)
	// }

	c.InvokeHook(ctx)
}

func (c *Comp) traceReadWriteEnd(req mem.AccessReq) {
	ctx := hooking.HookCtx{
		Domain: c,
		Pos:    hooking.HookPosTaskEnd,
		Item:   hooking.TaskEnd{ID: modeling.ReqOutTaskID(req.Meta().ID)},
	}

	// switch req := req.(type) {
	// case mem.ReadReq:
	// 	fmt.Printf("%.10f, %s, end read, 0x%016x\n",
	// 		c.Now(), c.Name(), req.Address)
	// case mem.WriteReq:
	// 	fmt.Printf("%.10f, %s, end write, 0x%016x, %v\n",
	// 		c.Now(), c.Name(), req.Address, req.Data)
	// }

	c.InvokeHook(ctx)
}
