package datamover

import (
	"log"
	"reflect"

	"github.com/sarchlab/akita/v5/mem/mem"
	"github.com/sarchlab/akita/v5/modeling"
	"github.com/sarchlab/akita/v5/sim"
	"github.com/sarchlab/akita/v5/tracing"
)

// Spec contains immutable configuration for the data mover.
type Spec struct {
	BufferSize             uint64 `json:"buffer_size"`
	InsideByteGranularity  uint64 `json:"inside_byte_granularity"`
	OutsideByteGranularity uint64 `json:"outside_byte_granularity"`
}

// State contains mutable runtime data for the data mover.
// For now this is minimal — the runtime transaction state with *sim.Msg
// pointers stays on Comp as non-serializable runtime fields.
type State struct{}

// A dataMoverTransaction contains a data moving request from a single
// source/destination with Read/Write requests correspond to it.
type dataMoverTransaction struct {
	req           *sim.Msg // payload: *DataMoveRequestPayload
	reqPayload    *DataMoveRequestPayload
	nextReadAddr  uint64
	nextWriteAddr uint64
	pendingRead   map[string]*sim.Msg // payload: *mem.ReadReqPayload
	pendingWrite  map[string]*sim.Msg // payload: *mem.WriteReqPayload
}

func newDataMoverTransaction(req *sim.Msg) *dataMoverTransaction {
	payload := sim.MsgPayload[DataMoveRequestPayload](req)
	return &dataMoverTransaction{
		req:           req,
		reqPayload:    payload,
		nextReadAddr:  payload.SrcAddress,
		nextWriteAddr: payload.DstAddress,
		pendingRead:   make(map[string]*sim.Msg),
		pendingWrite:  make(map[string]*sim.Msg),
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
	*modeling.Component[Spec, State]

	ctrlPort    sim.Port
	insidePort  sim.Port
	outsidePort sim.Port

	insidePortMapper  mem.AddressToPortMapper
	outsidePortMapper mem.AddressToPortMapper

	// Runtime-only fields (not serialized)
	srcPort            sim.Port
	dstPort            sim.Port
	srcPortMapper      mem.AddressToPortMapper
	dstPortMapper      mem.AddressToPortMapper
	srcByteGranularity uint64
	dstByteGranularity uint64
	currentTransaction *dataMoverTransaction
	buffer             *buffer
}

// dataMoverMiddleware wraps the Comp and implements the Tick() logic
// as a middleware.
type dataMoverMiddleware struct {
	*Comp
}

// Tick ticks
func (m *dataMoverMiddleware) Tick() bool {
	madeProgress := false

	madeProgress = m.finishTransaction() || madeProgress
	madeProgress = m.processWriteDoneFromDst() || madeProgress
	madeProgress = m.writeToDst() || madeProgress
	madeProgress = m.processDataReadyFromSrc() || madeProgress
	madeProgress = m.readFromSrc() || madeProgress
	madeProgress = m.parseFromCP() || madeProgress

	return madeProgress
}

// parseFromCP retrieves Msg from ctrlPort
func (m *dataMoverMiddleware) parseFromCP() bool {
	req := m.ctrlPort.RetrieveIncoming()
	if req == nil {
		return false
	}

	if m.currentTransaction != nil {
		return false
	}

	_, ok := req.Payload.(*DataMoveRequestPayload)
	if !ok {
		log.Panicf("can't process request of type %s", reflect.TypeOf(req.Payload))
	}

	trans := newDataMoverTransaction(req)
	m.currentTransaction = trans

	m.setSrcSide(trans.reqPayload)
	m.setDstSide(trans.reqPayload)

	m.buffer = &buffer{
		granularity: m.srcByteGranularity,
	}

	tracing.TraceReqReceive(req, m)

	return true
}

// readFromSrc reads data from source
func (m *dataMoverMiddleware) readFromSrc() bool {
	if m.currentTransaction == nil {
		return false
	}

	trans := m.currentTransaction
	addr := alignAddress(trans.nextReadAddr, m.srcByteGranularity)

	spec := m.Component.GetSpec()
	bufEndAddr := m.buffer.offset + spec.BufferSize
	if addr >= bufEndAddr {
		return false
	}

	transEndAddr := trans.reqPayload.SrcAddress + trans.reqPayload.ByteSize
	if addr > transEndAddr {
		return false
	}

	req := mem.ReadReqBuilder{}.
		WithAddress(addr).
		WithSrc(m.srcPort.AsRemote()).
		WithDst(m.srcPortMapper.Find(addr)).
		WithByteSize(m.srcByteGranularity).
		WithPID(0).
		Build()

	err := m.srcPort.Send(req)
	if err != nil {
		return false
	}

	trans.nextReadAddr += m.srcByteGranularity
	trans.pendingRead[req.ID] = req

	tracing.TraceReqInitiate(req, m, tracing.MsgIDAtReceiver(trans.req, m))

	return true
}

// processDataReadyFromSrc processes data ready from source
func (m *dataMoverMiddleware) processDataReadyFromSrc() bool {
	if m.currentTransaction == nil {
		return false
	}

	rsp := m.srcPort.PeekIncoming()
	if rsp == nil {
		return false
	}

	readRspPayload, ok := rsp.Payload.(*mem.DataReadyRspPayload)
	if !ok {
		// it can be write done rsp if src and dst is the same side. So ignore.
		return false
	}

	originalReq, ok := m.currentTransaction.pendingRead[rsp.RspTo]
	if !ok {
		log.Panicf("can't find original request for response %s",
			rsp.RspTo)
	}

	originalReqPayload := sim.MsgPayload[mem.ReadReqPayload](originalReq)
	offset := originalReqPayload.Address - m.currentTransaction.reqPayload.SrcAddress
	m.buffer.addData(offset, readRspPayload.Data)

	delete(m.currentTransaction.pendingRead, rsp.RspTo)
	m.srcPort.RetrieveIncoming()
	tracing.TraceReqFinalize(originalReq, m)

	return true
}

// writeToDst sends data to destination
func (m *dataMoverMiddleware) writeToDst() bool {
	if m.currentTransaction == nil {
		return false
	}

	trans := m.currentTransaction
	offset := trans.nextWriteAddr - trans.reqPayload.DstAddress
	data, ok := m.buffer.extractData(offset, m.dstByteGranularity)

	if !ok {
		return false
	}

	req := mem.WriteReqBuilder{}.
		WithAddress(m.currentTransaction.nextWriteAddr).
		WithData(data).
		WithSrc(m.dstPort.AsRemote()).
		WithDst(m.dstPortMapper.Find(m.currentTransaction.nextWriteAddr)).
		WithPID(0).
		Build()

	err := m.dstPort.Send(req)
	if err != nil {
		return false
	}

	m.currentTransaction.nextWriteAddr += m.dstByteGranularity
	m.currentTransaction.pendingWrite[req.ID] = req
	m.buffer.moveOffsetForwardTo(trans.nextWriteAddr - trans.reqPayload.DstAddress)

	tracing.TraceReqInitiate(req, m,
		tracing.MsgIDAtReceiver(m.currentTransaction.req, m))

	return true
}

// processWriteDoneFromDst processes write done from destination
func (m *dataMoverMiddleware) processWriteDoneFromDst() bool {
	if m.currentTransaction == nil {
		return false
	}

	rsp := m.dstPort.PeekIncoming()
	if rsp == nil {
		return false
	}

	_, ok := rsp.Payload.(*mem.WriteDoneRspPayload)
	if !ok {
		return false
	}

	originalReq, ok := m.currentTransaction.pendingWrite[rsp.RspTo]
	if !ok {
		log.Panicf("can't find original request for response %s",
			rsp.RspTo)
	}

	delete(m.currentTransaction.pendingWrite, rsp.RspTo)
	m.dstPort.RetrieveIncoming()

	tracing.TraceReqFinalize(originalReq, m)

	return false
}

// finishTransaction finishes the current transaction
func (m *dataMoverMiddleware) finishTransaction() bool {
	if m.currentTransaction == nil {
		return false
	}

	trans := m.currentTransaction

	if trans.nextWriteAddr < trans.reqPayload.DstAddress+trans.reqPayload.ByteSize {
		return false
	}

	rsp := &sim.Msg{
		MsgMeta: sim.MsgMeta{
			ID:  sim.GetIDGenerator().Generate(),
			Src: trans.req.Dst,
			Dst: trans.req.Src,
		},
		RspTo: trans.req.ID,
	}

	err := m.ctrlPort.Send(rsp)
	if err != nil {
		return false
	}

	m.currentTransaction = nil
	m.buffer = &buffer{
		offset:      alignAddress(trans.reqPayload.SrcAddress, m.srcByteGranularity),
		granularity: m.srcByteGranularity,
	}

	tracing.TraceReqComplete(rsp, m)

	return true
}

func (m *dataMoverMiddleware) setSrcSide(moveReq *DataMoveRequestPayload) {
	spec := m.Component.GetSpec()
	switch moveReq.SrcSide {
	case "inside":
		m.srcPort = m.insidePort
		m.srcPortMapper = m.insidePortMapper
		m.srcByteGranularity = spec.InsideByteGranularity
	case "outside":
		m.srcPort = m.outsidePort
		m.srcPortMapper = m.outsidePortMapper
		m.srcByteGranularity = spec.OutsideByteGranularity
	default:
		log.Panicf("can't process source port of type %s", moveReq.SrcSide)
	}

	addressMustBeAligned(moveReq.SrcAddress, m.srcByteGranularity)
}

func (m *dataMoverMiddleware) setDstSide(moveReq *DataMoveRequestPayload) {
	spec := m.Component.GetSpec()
	switch moveReq.DstSide {
	case "inside":
		m.dstPort = m.insidePort
		m.dstPortMapper = m.insidePortMapper
		m.dstByteGranularity = spec.InsideByteGranularity
	case "outside":
		m.dstPort = m.outsidePort
		m.dstPortMapper = m.outsidePortMapper
		m.dstByteGranularity = spec.OutsideByteGranularity
	default:
		log.Panicf("can't process destination port of type %s", moveReq.DstSide)
	}

	addressMustBeAligned(moveReq.DstAddress, m.dstByteGranularity)
}
